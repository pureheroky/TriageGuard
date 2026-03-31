package handlers

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
)

func parseWindowDays(raw string) int {
	windowDays := 30
	if parsed, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && (parsed == 7 || parsed == 30 || parsed == 90) {
		windowDays = parsed
	}
	return windowDays
}

func csvTimeValue(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.UTC().Format(time.RFC3339)
}

func csvFloatValue(v *float64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatFloat(*v, 'f', 2, 64)
}

func (a *App) workspaceHasEnterprise(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	allowed, plan, _, err := a.workspaceHasPaidAccess(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	return allowed && plan == planEnterprise, nil
}

func (a *App) handleGetMe(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	policy, _ := a.store.GetPolicy(r.Context(), workspace.ID)
	_, slackErr := a.store.GetSlackInstallationByWorkspace(r.Context(), workspace.ID)
	_, linearErr := a.store.GetLinearInstallation(r.Context(), workspace.ID)
	subscription, _ := a.store.GetWorkspaceSubscriptionOrDefault(r.Context(), workspace.ID)
	userID, _ := userIDFromContext(r.Context())
	jsonResponse(w, http.StatusOK, map[string]any{
		"workspace": workspace,
		"integrations": map[string]any{
			"slack_connected":  slackErr == nil,
			"linear_connected": linearErr == nil,
		},
		"policy": policy,
		"billing": map[string]any{
			"plan_key":             normalizePlanKey(subscription.PlanKey),
			"status":               strings.ToLower(strings.TrimSpace(subscription.Status)),
			"billing_provider":     normalizeBillingProvider(subscription.BillingProvider),
			"effective_plan":       effectivePlan(subscription),
			"cancel_at_period_end": subscription.CancelAtPeriodEnd,
			"current_period_end":   subscription.CurrentPeriodEnd,
			"entitlements":         entitlementsForPlan(effectivePlan(subscription)),
		},
		"internal_operator": a.isInternalOperatorUser(userID),
	})
}

func (a *App) handleListChannels(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	channels, err := a.store.ListChannels(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"channels": channels})
}

func (a *App) handleListQueues(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	queues, err := a.store.ListQueues(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	policies, err := a.store.ListQueuePolicies(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	policyByQueue := map[string]db.QueuePolicy{}
	for _, policy := range policies {
		policyByQueue[policy.QueueID.String()] = policy
	}
	items := make([]map[string]any, 0, len(queues))
	for _, queue := range queues {
		members, _ := a.store.ListQueueMembers(r.Context(), queue.ID)
		items = append(items, map[string]any{
			"queue":   queue,
			"policy":  policyByQueue[queue.ID.String()],
			"members": members,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"queues": items})
}

func (a *App) handleListQueueRoutingRules(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	rules, err := a.store.ListQueueRoutingRules(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"rules": rules})
}

func (a *App) handleUpsertQueueRoutingRules(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	var payload struct {
		Rules []struct {
			ID             string          `json:"id"`
			Name           string          `json:"name"`
			SortOrder      int             `json:"sort_order"`
			IsEnabled      bool            `json:"is_enabled"`
			ConditionsJSON json.RawMessage `json:"conditions_json"`
			QueueID        string          `json:"queue_id"`
			RequestType    *string         `json:"request_type"`
			Priority       *string         `json:"priority"`
			StopProcessing bool            `json:"stop_processing"`
		} `json:"rules"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	rules := make([]db.QueueRoutingRule, 0, len(payload.Rules))
	for _, item := range payload.Rules {
		queueID, err := uuid.Parse(strings.TrimSpace(item.QueueID))
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid queue_id for rule %s", item.Name)})
			return
		}
		var ruleID uuid.UUID
		if strings.TrimSpace(item.ID) != "" {
			ruleID, err = uuid.Parse(strings.TrimSpace(item.ID))
			if err != nil {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid rule id: %s", item.ID)})
				return
			}
		}
		rules = append(rules, db.QueueRoutingRule{
			ID:             ruleID,
			WorkspaceID:    workspace.ID,
			Name:           item.Name,
			SortOrder:      item.SortOrder,
			IsEnabled:      item.IsEnabled,
			ConditionsJSON: item.ConditionsJSON,
			QueueID:        queueID,
			RequestType:    item.RequestType,
			Priority:       item.Priority,
			StopProcessing: item.StopProcessing,
		})
	}
	if err := a.store.ReplaceQueueRoutingRules(r.Context(), workspace.ID, rules); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	a.handleListQueueRoutingRules(w, r)
}

func (a *App) handleListEscalationSteps(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	queueID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("queue_id")))
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "invalid queue_id"})
		return
	}
	queue, err := a.store.GetQueueByID(r.Context(), queueID)
	if err != nil || queue.WorkspaceID != workspace.ID {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "queue not found"})
		return
	}
	steps, err := a.store.ListEscalationStepsByQueue(r.Context(), queueID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"steps": steps})
}

func (a *App) handleUpsertEscalationSteps(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{"error": "Enterprise plan required for custom escalation steps."})
		return
	}
	var payload struct {
		QueueID string `json:"queue_id"`
		Steps   []struct {
			ID                      string  `json:"id"`
			ClockType               string  `json:"clock_type"`
			DelayMinutesAfterBreach int     `json:"delay_minutes_after_breach"`
			TargetType              string  `json:"target_type"`
			TargetValue             *string `json:"target_value"`
			MessageTemplate         *string `json:"message_template"`
			IsEnabled               bool    `json:"is_enabled"`
		} `json:"steps"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	queueID, err := uuid.Parse(strings.TrimSpace(payload.QueueID))
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "invalid queue_id"})
		return
	}
	queue, err := a.store.GetQueueByID(r.Context(), queueID)
	if err != nil || queue.WorkspaceID != workspace.ID {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "queue not found"})
		return
	}
	steps := make([]db.EscalationStep, 0, len(payload.Steps))
	for _, item := range payload.Steps {
		var stepID uuid.UUID
		if strings.TrimSpace(item.ID) != "" {
			stepID, err = uuid.Parse(strings.TrimSpace(item.ID))
			if err != nil {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid escalation step id: %s", item.ID)})
				return
			}
		}
		steps = append(steps, db.EscalationStep{
			ID:                      stepID,
			QueueID:                 queueID,
			ClockType:               item.ClockType,
			DelayMinutesAfterBreach: item.DelayMinutesAfterBreach,
			TargetType:              item.TargetType,
			TargetValue:             item.TargetValue,
			MessageTemplate:         item.MessageTemplate,
			IsEnabled:               item.IsEnabled,
		})
	}
	if err := a.store.ReplaceEscalationSteps(r.Context(), queueID, steps); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	r.URL.RawQuery = url.Values{"queue_id": []string{queueID.String()}}.Encode()
	a.handleListEscalationSteps(w, r)
}

func (a *App) handleUpsertQueues(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	var payload struct {
		Queues []struct {
			ID                  string `json:"id"`
			Name                string `json:"name"`
			Description         string `json:"description"`
			DefaultPriority     string `json:"default_priority"`
			DefaultRequestType  string `json:"default_request_type"`
			EscalationChannelID string `json:"escalation_channel_id"`
			IsDefault           bool   `json:"is_default"`
			Policy              struct {
				AckSLAMinutes        int    `json:"ack_sla_minutes"`
				AssignSLAMinutes     int    `json:"assign_sla_minutes"`
				StaleHours           int    `json:"stale_hours"`
				DigestTime           string `json:"digest_time"`
				DigestWeekday        *int   `json:"digest_weekday"`
				AssignStartsFrom     string `json:"assign_starts_from"`
				StaleStartsFrom      string `json:"stale_starts_from"`
				Timezone             string `json:"timezone"`
				BusinessHoursEnabled bool   `json:"business_hours_enabled"`
				BusinessHoursStart   string `json:"business_hours_start"`
				BusinessHoursEnd     string `json:"business_hours_end"`
				BusinessDaysMask     int    `json:"business_days_mask"`
			} `json:"policy"`
			Members []struct {
				UserID string `json:"user_id"`
				Role   string `json:"role"`
			} `json:"members"`
		} `json:"queues"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	entitlements, err := a.workspaceEntitlements(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	existingQueues, err := a.store.ListQueues(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	queuedUpdates := map[string][]struct {
		UserID string
		Role   string
	}{}
	for _, item := range payload.Queues {
		key := strings.TrimSpace(item.ID)
		queuedUpdates[key] = make([]struct {
			UserID string
			Role   string
		}, 0, len(item.Members))
		for _, member := range item.Members {
			queuedUpdates[key] = append(queuedUpdates[key], struct {
				UserID string
				Role   string
			}{UserID: strings.TrimSpace(member.UserID), Role: strings.TrimSpace(member.Role)})
		}
	}
	if entitlements.TriagerLimit != nil {
		allTriagers := map[string]struct{}{}
		for _, queue := range existingQueues {
			key := queue.ID.String()
			if replacements, ok := queuedUpdates[key]; ok {
				for _, member := range replacements {
					if member.UserID != "" && strings.EqualFold(member.Role, "triager") {
						allTriagers[member.UserID] = struct{}{}
					}
				}
				delete(queuedUpdates, key)
				continue
			}
			members, _ := a.store.ListQueueMembers(r.Context(), queue.ID)
			for _, member := range members {
				if strings.EqualFold(member.Role, "triager") && strings.TrimSpace(member.UserID) != "" {
					allTriagers[strings.TrimSpace(member.UserID)] = struct{}{}
				}
			}
		}
		for _, replacements := range queuedUpdates {
			for _, member := range replacements {
				if member.UserID != "" && strings.EqualFold(member.Role, "triager") {
					allTriagers[member.UserID] = struct{}{}
				}
			}
		}
		if len(allTriagers) > *entitlements.TriagerLimit {
			jsonResponse(w, http.StatusPaymentRequired, map[string]any{
				"error": fmt.Sprintf("Team plan supports up to %d triagers across all queues. Upgrade in Billing to add more triagers.", *entitlements.TriagerLimit),
			})
			return
		}
	}

	var sharedPolicy db.Policy
	if !hasEnterprise {
		sharedPolicy, err = a.store.GetPolicy(r.Context(), workspace.ID)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}

	for _, item := range payload.Queues {
		var queueID uuid.UUID
		if strings.TrimSpace(item.ID) != "" {
			parsed, err := uuid.Parse(strings.TrimSpace(item.ID))
			if err != nil {
				jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid queue id: %s", item.ID)})
				return
			}
			queueID = parsed
		}
		queue, err := a.store.UpsertQueue(r.Context(), db.Queue{
			ID:                  queueID,
			WorkspaceID:         workspace.ID,
			Name:                strings.TrimSpace(item.Name),
			Description:         optionalString(item.Description),
			DefaultPriority:     item.DefaultPriority,
			DefaultRequestType:  item.DefaultRequestType,
			EscalationChannelID: optionalString(item.EscalationChannelID),
			IsDefault:           item.IsDefault,
		})
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		queuePolicy := db.QueuePolicy{
			QueueID:              queue.ID,
			AckSLAMinutes:        item.Policy.AckSLAMinutes,
			AssignSLAMinutes:     item.Policy.AssignSLAMinutes,
			StaleHours:           item.Policy.StaleHours,
			DigestTime:           item.Policy.DigestTime,
			DigestWeekday:        item.Policy.DigestWeekday,
			AssignStartsFrom:     item.Policy.AssignStartsFrom,
			StaleStartsFrom:      item.Policy.StaleStartsFrom,
			Timezone:             item.Policy.Timezone,
			BusinessHoursEnabled: item.Policy.BusinessHoursEnabled,
			BusinessHoursStart:   optionalString(item.Policy.BusinessHoursStart),
			BusinessHoursEnd:     optionalString(item.Policy.BusinessHoursEnd),
			BusinessDaysMask:     item.Policy.BusinessDaysMask,
		}
		if !hasEnterprise {
			queuePolicy = db.QueuePolicy{
				QueueID:              queue.ID,
				AckSLAMinutes:        sharedPolicy.AckSLAMinutes,
				AssignSLAMinutes:     sharedPolicy.AssignSLAMinutes,
				StaleHours:           sharedPolicy.StaleHours,
				DigestTime:           sharedPolicy.DailyDigestTime,
				DigestWeekday:        nil,
				AssignStartsFrom:     "created_at",
				StaleStartsFrom:      "last_human_activity_at",
				Timezone:             sharedPolicy.Timezone,
				BusinessHoursEnabled: false,
				BusinessHoursStart:   nil,
				BusinessHoursEnd:     nil,
				BusinessDaysMask:     62,
			}
		}
		if _, err := a.store.UpsertQueuePolicy(r.Context(), queuePolicy); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		members := make([]db.QueueMember, 0, len(item.Members))
		for _, member := range item.Members {
			members = append(members, db.QueueMember{
				QueueID: queue.ID,
				UserID:  member.UserID,
				Role:    member.Role,
			})
		}
		if err := a.store.ReplaceQueueMembers(r.Context(), queue.ID, members); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	a.handleListQueues(w, r)
}

func (a *App) handleGetQueueDetail(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	queueID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "queueID")))
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "invalid queue id"})
		return
	}
	detail, err := a.store.GetQueueDetailMetrics(r.Context(), queueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonResponse(w, http.StatusNotFound, map[string]any{"error": "queue not found"})
			return
		}
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if detail.Queue.WorkspaceID != workspace.ID {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "queue not found"})
		return
	}
	jsonResponse(w, http.StatusOK, detail)
}

func (a *App) handleSlackUsers(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	install, err := a.store.GetSlackInstallationByWorkspace(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "slack is not connected"})
		return
	}
	users, err := a.slackClient.ListUsers(r.Context(), install.BotToken)
	if err != nil {
		jsonResponse(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"users": users})
}

func (a *App) handleSyncChannels(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	install, err := a.store.GetSlackInstallationByWorkspace(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "slack is not connected"})
		return
	}
	channels, err := a.slackClient.ListConversations(r.Context(), install.BotToken)
	if err != nil {
		jsonResponse(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	rows := make([]db.SlackChannel, 0, len(channels))
	for _, ch := range channels {
		rows = append(rows, db.SlackChannel{
			ChannelID:   ch.ID,
			ChannelName: ch.Name,
			IsPrivate:   ch.IsPrivate,
		})
	}
	if err := a.store.UpsertChannels(r.Context(), workspace.ID, rows); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	updated, _ := a.store.ListChannels(r.Context(), workspace.ID)
	jsonResponse(w, http.StatusOK, map[string]any{"channels": updated})
}

func (a *App) handleUpdateChannels(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	var payload struct {
		Channels []struct {
			ChannelID      string `json:"channel_id"`
			Enabled        bool   `json:"enabled"`
			DefaultQueueID string `json:"default_queue_id"`
		} `json:"channels"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	updates := map[string]bool{}
	defaultQueues := map[string]*uuid.UUID{}
	for _, item := range payload.Channels {
		updates[strings.TrimSpace(item.ChannelID)] = item.Enabled
		if strings.TrimSpace(item.DefaultQueueID) == "" {
			defaultQueues[strings.TrimSpace(item.ChannelID)] = nil
			continue
		}
		queueID, err := uuid.Parse(strings.TrimSpace(item.DefaultQueueID))
		if err != nil {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("invalid default_queue_id for channel %s", item.ChannelID)})
			return
		}
		defaultQueues[strings.TrimSpace(item.ChannelID)] = &queueID
	}

	subscription, err := a.store.GetWorkspaceSubscriptionOrDefault(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	plan := effectivePlan(subscription)
	if limit := channelLimitForPlan(plan); limit != nil {
		currentChannels, err := a.store.ListChannels(r.Context(), workspace.ID)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		enabledCount := 0
		for _, channel := range currentChannels {
			enabled := channel.Enabled
			if next, ok := updates[channel.ChannelID]; ok {
				enabled = next
			}
			if enabled {
				enabledCount++
			}
		}
		if enabledCount > *limit {
			planLabel := strings.ToUpper(plan)
			jsonResponse(w, http.StatusPaymentRequired, map[string]any{
				"error": fmt.Sprintf("%s plan supports up to %d enabled channels. Upgrade in Billing to enable more channels.", planLabel, *limit),
			})
			return
		}
	}

	if err := a.store.UpdateChannelStates(r.Context(), workspace.ID, updates); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	for channelID, queueID := range defaultQueues {
		if err := a.store.SetChannelDefaultQueue(r.Context(), workspace.ID, channelID, queueID); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	channels, _ := a.store.ListChannels(r.Context(), workspace.ID)
	jsonResponse(w, http.StatusOK, map[string]any{"channels": channels})
}

func (a *App) handleGetChannelOverrides(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	overrides, err := a.store.ListChannelSLAOverrides(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	canEdit, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"overrides": overrides,
		"can_edit":  canEdit,
	})
}

func (a *App) handleUpdateChannelOverrides(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "Enterprise plan required for per-channel SLA overrides.",
		})
		return
	}

	var payload struct {
		Overrides []struct {
			ChannelID        string `json:"channel_id"`
			AckSLAMinutes    int    `json:"ack_sla_minutes"`
			AssignSLAMinutes int    `json:"assign_sla_minutes"`
			StaleHours       int    `json:"stale_hours"`
		} `json:"overrides"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	channels, err := a.store.ListChannels(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	knownChannels := make(map[string]struct{}, len(channels))
	for _, channel := range channels {
		knownChannels[channel.ChannelID] = struct{}{}
	}

	seen := map[string]struct{}{}
	overrides := make([]db.ChannelSLAOverride, 0, len(payload.Overrides))
	for _, item := range payload.Overrides {
		channelID := strings.TrimSpace(item.ChannelID)
		if channelID == "" {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "channel_id is required"})
			return
		}
		if _, ok := knownChannels[channelID]; !ok {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("unknown channel_id: %s", channelID)})
			return
		}
		if _, exists := seen[channelID]; exists {
			jsonResponse(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("duplicate override for channel_id: %s", channelID)})
			return
		}
		seen[channelID] = struct{}{}
		if item.AckSLAMinutes <= 0 || item.AssignSLAMinutes <= 0 || item.StaleHours <= 0 {
			jsonResponse(w, http.StatusBadRequest, map[string]any{
				"error": "ack_sla_minutes, assign_sla_minutes, stale_hours must be positive for each override",
			})
			return
		}
		overrides = append(overrides, db.ChannelSLAOverride{
			WorkspaceID:      workspace.ID,
			ChannelID:        channelID,
			AckSLAMinutes:    item.AckSLAMinutes,
			AssignSLAMinutes: item.AssignSLAMinutes,
			StaleHours:       item.StaleHours,
		})
	}

	if err := a.store.ReplaceChannelSLAOverrides(r.Context(), workspace.ID, overrides); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	updated, err := a.store.ListChannelSLAOverrides(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"overrides": updated,
		"can_edit":  true,
	})
}

func (a *App) handleDisconnectSlack(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	disconnected, err := a.store.DisconnectSlack(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":           true,
		"disconnected": disconnected,
	})
}

func (a *App) handleDeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	var payload struct {
		Confirm string `json:"confirm"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	if strings.TrimSpace(payload.Confirm) != "DELETE" {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "confirm must be DELETE"})
		return
	}

	if err := a.store.DeleteWorkspace(r.Context(), workspace.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
			return
		}
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	a.logger.Printf("workspace deleted workspace_id=%s", workspace.ID)

	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "deleted": true})
}

func (a *App) handleGetPolicies(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	policy, err := a.store.GetPolicy(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, policy)
}

func (a *App) handleUpdatePolicies(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	var payload struct {
		AckSLAMinutes       int    `json:"ack_sla_minutes"`
		AssignSLAMinutes    int    `json:"assign_sla_minutes"`
		StaleHours          int    `json:"stale_hours"`
		DailyDigestTime     string `json:"daily_digest_time"`
		Timezone            string `json:"timezone"`
		EscalationChannelID string `json:"escalation_channel_id"`
		LinearTeamID        string `json:"linear_team_id"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if payload.AckSLAMinutes <= 0 {
		payload.AckSLAMinutes = 15
	}
	if payload.AssignSLAMinutes <= 0 {
		payload.AssignSLAMinutes = 30
	}
	if payload.StaleHours <= 0 {
		payload.StaleHours = 24
	}
	if payload.DailyDigestTime == "" {
		payload.DailyDigestTime = "09:00:00"
	}
	normalizedDigestTime, err := parseTimeOfDay(payload.DailyDigestTime)
	if err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "invalid daily_digest_time, expected HH:MM or HH:MM:SS"})
		return
	}
	payload.DailyDigestTime = normalizedDigestTime
	if payload.Timezone == "" {
		payload.Timezone = "UTC"
	}
	policy := db.Policy{
		WorkspaceID:         workspace.ID,
		AckSLAMinutes:       payload.AckSLAMinutes,
		AssignSLAMinutes:    payload.AssignSLAMinutes,
		StaleHours:          payload.StaleHours,
		DailyDigestTime:     payload.DailyDigestTime,
		Timezone:            payload.Timezone,
		EscalationChannelID: optionalString(payload.EscalationChannelID),
		LinearTeamID:        optionalString(payload.LinearTeamID),
	}
	if err := a.store.UpdatePolicy(r.Context(), policy); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		if err := a.store.SyncWorkspacePolicyToQueues(r.Context(), workspace.ID, policy); err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	}
	updated, _ := a.store.GetPolicy(r.Context(), workspace.ID)
	jsonResponse(w, http.StatusOK, updated)
}

func (a *App) handleGetLinear(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	policy, _ := a.store.GetPolicy(r.Context(), workspace.ID)
	_, err = a.store.GetLinearInstallation(r.Context(), workspace.ID)
	if err != nil && !errorsIsNoRows(err) {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	installed := err == nil
	connected := installed
	requiresReconnect := false
	connectionError := ""

	if installed {
		healthCtx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		healthErr := a.withLinearAccessToken(healthCtx, workspace.ID, func(token string) error {
			_, _, err := a.linearClient.GetViewer(healthCtx, token)
			return err
		})
		if healthErr != nil {
			if linear.IsAuthError(healthErr) || strings.Contains(strings.ToLower(healthErr.Error()), "linear auth required") {
				connected = false
				requiresReconnect = true
				connectionError = "Linear authorization expired or revoked. Reconnect Linear."
			} else {
				connectionError = "Linear health check failed. Please retry."
				a.logger.Printf("linear health check failed workspace=%s: %v", workspace.ID, healthErr)
			}
		}
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"installed":          installed,
		"connected":          connected,
		"requires_reconnect": requiresReconnect,
		"connection_error":   connectionError,
		"linear_team":        policy.LinearTeamID,
		"oauth_url":          strings.TrimSuffix(a.cfg.APIBaseURL, "/") + "/api/linear/install",
	})
}

func (a *App) handleSetLinearDefaults(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	var payload struct {
		LinearTeamID string `json:"linear_team_id"`
	}
	if err := readJSON(r, &payload); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	policy, err := a.store.GetPolicy(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	policy.LinearTeamID = optionalString(payload.LinearTeamID)
	if err := a.store.UpdatePolicy(r.Context(), policy); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleDisconnectLinear(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}

	disconnected, err := a.store.DisconnectLinear(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{
		"ok":           true,
		"disconnected": disconnected,
	})
}

func (a *App) handleListLinearTeams(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	if _, err := a.store.GetLinearInstallation(r.Context(), workspace.ID); err != nil {
		jsonResponse(w, http.StatusBadRequest, map[string]any{"error": "linear is not connected"})
		return
	}
	var teams []linear.Team
	if err := a.withLinearAccessToken(r.Context(), workspace.ID, func(token string) error {
		fetchedTeams, err := a.linearClient.ListTeams(r.Context(), token)
		if err != nil {
			return err
		}
		teams = fetchedTeams
		return nil
	}); err != nil {
		if linear.IsAuthError(err) || strings.Contains(strings.ToLower(err.Error()), "linear auth required") {
			jsonResponse(w, http.StatusConflict, map[string]any{"error": "linear authorization expired, reconnect Linear"})
			return
		}
		jsonResponse(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"teams": teams})
}

func (a *App) handleRequestSummary(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	dashboard, err := a.store.QueueDashboardAnalytics(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, dashboard)
}

func (a *App) handleListRequests(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	requests, err := a.store.ListRequestsByStatus(r.Context(), workspace.ID, status, limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	queueCache := map[uuid.UUID]db.Queue{}
	type requestDTO struct {
		Request        db.Request           `json:"request"`
		DerivedStatus  string               `json:"derived_status"`
		Queue          *db.Queue            `json:"queue,omitempty"`
		ClockStates    []db.RequestSLAClock `json:"clock_states"`
		ExternalIssues []db.ExternalIssue   `json:"external_issues"`
	}
	items := make([]requestDTO, 0, len(requests))
	for _, request := range requests {
		var queue *db.Queue
		if request.QueueID != nil {
			if cached, ok := queueCache[*request.QueueID]; ok {
				queue = &cached
			} else if loaded, err := a.store.GetQueueByID(r.Context(), *request.QueueID); err == nil {
				queueCache[loaded.ID] = loaded
				queue = &loaded
			}
		}
		clocks, _ := a.store.ListRequestSLAClocks(r.Context(), request.ID)
		externalIssues, _ := a.store.ListExternalIssuesByRequest(r.Context(), request.ID)
		items = append(items, requestDTO{
			Request:        request,
			DerivedStatus:  db.DerivedStatus(request),
			Queue:          queue,
			ClockStates:    clocks,
			ExternalIssues: externalIssues,
		})
	}
	jsonResponse(w, http.StatusOK, map[string]any{"requests": items})
}

func (a *App) handleActivity(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	activity, err := a.store.ListActivity(r.Context(), workspace.ID, limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"activity": activity})
}

func (a *App) handleListDeadLetters(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	limit := 100
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil {
			limit = parsed
		}
	}
	rows, err := a.store.ListDeadLetters(r.Context(), workspace.ID, limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"dead_letters": rows})
}

func (a *App) handleRequestsCSV(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "Enterprise plan required for CSV exports.",
		})
		return
	}

	windowDays := parseWindowDays(r.URL.Query().Get("days"))
	rows, err := a.store.ListRequestReportRows(r.Context(), workspace.ID, windowDays)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	lookup := a.buildRequestReportLookup(r.Context(), workspace.ID, rows)

	filename := fmt.Sprintf("triageguard-requests-%dd.csv", windowDays)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"request_id",
		"channel",
		"channel_id",
		"thread_ts",
		"thread_url",
		"title",
		"status",
		"priority",
		"owner",
		"owner_slack_id",
		"created_at",
		"acked_at",
		"assigned_at",
		"resolved_at",
		"due_at",
		"ack_overdue_sent_at",
		"assign_overdue_sent_at",
		"stale_sent_at",
		"linear_issue",
		"linear_issue_url",
	})
	for _, item := range rows {
		threadURL := ""
		if item.ThreadURL != nil {
			threadURL = *item.ThreadURL
		}
		title := ""
		if item.Title != nil {
			title = *item.Title
		}
		owner := lookup.ownerDisplay(item.OwnerSlackID)
		ownerID := ""
		if item.OwnerSlackID != nil {
			ownerID = strings.TrimSpace(*item.OwnerSlackID)
		}
		linearLabel := linearIssueLabel(item.LinearIssueURL, item.LinearIssueID)
		linearURL := ""
		if item.LinearIssueURL != nil {
			linearURL = *item.LinearIssueURL
		}
		_ = cw.Write([]string{
			item.ID.String(),
			lookup.channelDisplay(item.ChannelID),
			item.ChannelID,
			item.ThreadTS,
			threadURL,
			title,
			item.Status,
			item.Priority,
			owner,
			ownerID,
			item.CreatedAt.UTC().Format(time.RFC3339),
			csvTimeValue(item.AckedAt),
			csvTimeValue(item.AssignedAt),
			csvTimeValue(item.ResolvedAt),
			csvTimeValue(item.DueAt),
			csvTimeValue(item.AckOverdueSentAt),
			csvTimeValue(item.AssignOverdueSentAt),
			csvTimeValue(item.StaleSentAt),
			linearLabel,
			linearURL,
		})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		a.logger.Printf("requests csv write error workspace=%s: %v", workspace.ID, err)
	}
}

func (a *App) handleAnalyticsCSV(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if !hasEnterprise {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "Enterprise plan required for CSV exports.",
		})
		return
	}

	windowDays := parseWindowDays(r.URL.Query().Get("days"))
	timezone := "UTC"
	if policy, policyErr := a.store.GetPolicy(r.Context(), workspace.ID); policyErr == nil && strings.TrimSpace(policy.Timezone) != "" {
		if _, tzErr := time.LoadLocation(policy.Timezone); tzErr == nil {
			timezone = policy.Timezone
		}
	}
	analytics, err := a.store.RequestAnalytics(r.Context(), workspace.ID, windowDays, timezone)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("triageguard-analytics-%dd.csv", windowDays)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Cache-Control", "no-store")

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{
		"date",
		"window_days",
		"ack_avg_minutes",
		"ack_sample_size",
		"resolve_avg_minutes",
		"resolve_sample_size",
	})
	for _, point := range analytics.Trend {
		_ = cw.Write([]string{
			point.Date,
			strconv.Itoa(analytics.WindowDays),
			csvFloatValue(point.AckAvgMinutes),
			strconv.Itoa(point.AckSampleSize),
			csvFloatValue(point.ResolveAvgMinutes),
			strconv.Itoa(point.ResolveSampleSize),
		})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		a.logger.Printf("analytics csv write error workspace=%s: %v", workspace.ID, err)
	}
}

func (a *App) handleLinearInstall(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	userID, err := userIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	state, nonce, err := a.buildOAuthState("linear", &userID, &workspace.ID)
	if err != nil {
		http.Error(w, "failed to start linear oauth", http.StatusInternalServerError)
		return
	}
	a.setOAuthStateCookie(w, "linear", nonce)

	u, _ := url.Parse("https://linear.app/oauth/authorize")
	q := u.Query()
	q.Set("client_id", a.cfg.LinearClientID)
	q.Set("redirect_uri", a.cfg.LinearRedirectURI)
	q.Set("response_type", "code")
	q.Set("scope", "read,write")
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (a *App) handleLinearOAuthCallback(w http.ResponseWriter, r *http.Request) {
	defer a.clearOAuthStateCookie(w, "linear")
	statePayload, err := a.validateOAuthState(r, "linear")
	if err != nil {
		a.logger.Printf("linear oauth callback: invalid state: %v", err)
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	if statePayload.WorkspaceID == "" {
		http.Error(w, "missing workspace in oauth state", http.StatusBadRequest)
		return
	}
	workspaceID, err := uuid.Parse(statePayload.WorkspaceID)
	if err != nil {
		http.Error(w, "invalid workspace in oauth state", http.StatusBadRequest)
		return
	}

	token, err := a.linearClient.ExchangeOAuthCode(r.Context(), a.cfg.LinearClientID, a.cfg.LinearClientSecret, code, a.cfg.LinearRedirectURI)
	if err != nil {
		a.logger.Printf("linear oauth callback: exchange failed workspace=%s: %v", workspaceID, err)
		http.Error(w, "linear oauth exchange failed", http.StatusBadGateway)
		return
	}
	if err := a.store.UpsertLinearInstallation(
		r.Context(),
		workspaceID,
		token.AccessToken,
		optionalString(token.RefreshToken),
		linearTokenExpiresAt(time.Now(), token.ExpiresIn),
	); err != nil {
		a.logger.Printf("linear oauth callback: save installation failed workspace=%s: %v", workspaceID, err)
		http.Error(w, "failed to save linear installation", http.StatusInternalServerError)
		return
	}
	if statePayload.UserID != "" {
		userID, err := uuid.Parse(statePayload.UserID)
		if err != nil {
			http.Error(w, "invalid oauth state user", http.StatusBadRequest)
			return
		}
		if err := a.store.EnsureWorkspaceMember(r.Context(), workspaceID, userID, "admin"); err != nil {
			a.logger.Printf("linear oauth callback: ensure workspace member failed workspace=%s user=%s: %v", workspaceID, userID, err)
			http.Error(w, "failed to bind workspace member", http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, strings.TrimSuffix(a.cfg.WebBaseURL, "/")+"/linear?connected=1", http.StatusFound)
}

func (a *App) authContext(r *http.Request) context.Context {
	return r.Context()
}

func encodeJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func parseTimeOfDay(raw string) (string, error) {
	if raw == "" {
		return "09:00:00", nil
	}
	if len(raw) == 5 {
		raw += ":00"
	}
	if _, err := time.Parse("15:04:05", raw); err != nil {
		return "", err
	}
	return raw, nil
}
