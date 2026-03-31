package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
	"triageguard/apps/api/internal/slack"
)

type SLARunner struct {
	cfg          config.Config
	store        *db.Store
	slackClient  *slack.Client
	linearClient *linear.Client
}

const slaRunnerLockKey int64 = 81273451

func NewSLARunner(cfg config.Config, store *db.Store, slackClient *slack.Client, linearClient *linear.Client) *SLARunner {
	return &SLARunner{cfg: cfg, store: store, slackClient: slackClient, linearClient: linearClient}
}

func (r *SLARunner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	if err := r.RunOnce(ctx); err != nil {
		log.Printf("sla: initial run failed: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.RunOnce(ctx); err != nil {
				log.Printf("sla: periodic run failed: %v", err)
			}
		}
	}
}

func (r *SLARunner) RunOnce(ctx context.Context) error {
	startedAt := time.Now().UTC()
	_ = r.store.RecordJobRuntimeStart(ctx, "sla_runner", startedAt)
	var runErr error
	defer func() {
		_ = r.store.RecordJobRuntimeFinish(context.Background(), "sla_runner", time.Now().UTC(), time.Since(startedAt), runErr)
	}()

	locked, err := r.store.TryAdvisoryLock(ctx, slaRunnerLockKey)
	if err != nil {
		runErr = err
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.store.AdvisoryUnlock(unlockCtx, slaRunnerLockKey); err != nil {
			log.Printf("sla: advisory unlock failed: %v", err)
		}
	}()

	paidAccessCache := map[uuid.UUID]bool{}
	now := time.Now().UTC()
	if err := r.evaluateClocks(ctx, now, paidAccessCache); err != nil {
		runErr = err
		return err
	}
	if err := r.enqueueQueueDigests(ctx, now, paidAccessCache); err != nil {
		runErr = err
		return err
	}
	if err := r.enqueueManagerWeeklyDigests(ctx, now, paidAccessCache); err != nil {
		runErr = err
		return err
	}
	if err := r.refreshDailyMetrics(ctx, now); err != nil {
		runErr = err
		return err
	}
	return nil
}

func (r *SLARunner) workspaceHasPaidAccess(ctx context.Context, workspaceID uuid.UUID, cache map[uuid.UUID]bool) (bool, error) {
	if allowed, ok := cache[workspaceID]; ok {
		return allowed, nil
	}
	sub, err := r.store.GetWorkspaceSubscriptionOrDefault(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	status := strings.ToLower(strings.TrimSpace(sub.Status))
	isPaidStatus := status == "active" || status == "trialing" || status == "past_due" || status == "unpaid" || status == "incomplete"
	plan := strings.ToLower(strings.TrimSpace(sub.PlanKey))
	if plan == "pro" {
		plan = "enterprise"
	}
	if plan == "starter" {
		plan = "team"
	}
	allowed := (plan == "team" || plan == "enterprise") && isPaidStatus
	cache[workspaceID] = allowed
	return allowed, nil
}

func (r *SLARunner) evaluateClocks(ctx context.Context, now time.Time, paidAccessCache map[uuid.UUID]bool) error {
	requests, err := r.store.ListOpenRequestsForClockEvaluation(ctx)
	if err != nil {
		return err
	}

	queueCache := map[uuid.UUID]db.Queue{}
	for _, request := range requests {
		allowed, err := r.workspaceHasPaidAccess(ctx, request.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: paid access lookup failed workspace=%s: %v", request.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}

		req, clocks, err := r.store.ReconcileRequestClocks(ctx, request.ID, now)
		if err != nil {
			log.Printf("sla: reconcile clocks failed request_id=%s: %v", request.ID, err)
			continue
		}
		if req.QueueID == nil || db.NormalizeRequestState(req.State) != db.RequestStateOpen {
			continue
		}

		queue, ok := queueCache[*req.QueueID]
		if !ok {
			queue, err = r.store.GetQueueByID(ctx, *req.QueueID)
			if err != nil {
				log.Printf("sla: queue lookup failed request_id=%s queue_id=%s: %v", req.ID, *req.QueueID, err)
				continue
			}
			queueCache[queue.ID] = queue
		}

		for _, clock := range clocks {
			if clock.TargetAt == nil {
				continue
			}
			switch strings.TrimSpace(clock.State) {
			case "running":
				if now.Before(*clock.TargetAt) {
					continue
				}
				breachedClock, err := r.store.MarkClockBreached(ctx, clock.ID, now)
				if err != nil {
					log.Printf("sla: mark breached failed request_id=%s clock=%s: %v", req.ID, clock.ClockType, err)
					continue
				}
				clock = breachedClock
				payload, _ := json.Marshal(map[string]any{
					"clock_type": clock.ClockType,
					"target_at":  clock.TargetAt,
					"queue_id":   req.QueueID.String(),
				})
				if _, err := r.store.InsertRequestEvent(ctx, db.RequestEvent{
					RequestID:  req.ID,
					EventType:  "sla_breached",
					ActorType:  "scheduler",
					OccurredAt: now,
					Payload:    payload,
				}); err != nil {
					log.Printf("sla: event insert failed request_id=%s clock=%s: %v", req.ID, clock.ClockType, err)
				}
				if _, err := r.store.InsertOutboxItem(ctx, db.NotificationOutboxItem{
					RequestID:       &req.ID,
					Kind:            "triage_card_refresh",
					DestinationType: "request",
					Payload:         mustJSON(map[string]any{"request_id": req.ID.String()}),
					DedupeKey:       stringPtr(fmt.Sprintf("triage_card_refresh:%s", req.ID)),
					Status:          "pending",
					AvailableAt:     now,
				}); err != nil {
					log.Printf("sla: enqueue refresh failed request_id=%s: %v", req.ID, err)
				}
			case "breached":
			default:
				continue
			}

			if clock.BreachedAt == nil {
				continue
			}
			steps, err := r.store.ListEscalationSteps(ctx, queue.ID, clock.ClockType)
			if err != nil {
				log.Printf("sla: escalation steps lookup failed queue_id=%s clock=%s: %v", queue.ID, clock.ClockType, err)
				continue
			}
			for _, step := range steps {
				eligibleAt := clock.BreachedAt.Add(time.Duration(step.DelayMinutesAfterBreach) * time.Minute)
				if now.Before(eligibleAt) {
					continue
				}
				items := r.buildEscalationOutboxItems(ctx, req, queue, clock, step, now)
				for _, item := range items {
					if _, err := r.store.InsertOutboxItem(ctx, item); err != nil {
						log.Printf("sla: enqueue escalation failed request_id=%s step_id=%s: %v", req.ID, step.ID, err)
					}
				}
			}
		}
	}
	return nil
}

func (r *SLARunner) buildEscalationOutboxItems(ctx context.Context, req db.Request, queue db.Queue, clock db.RequestSLAClock, step db.EscalationStep, now time.Time) []db.NotificationOutboxItem {
	targets := make([]string, 0, 4)
	switch step.TargetType {
	case "thread":
		targets = append(targets, "")
	case "channel":
		if step.TargetValue != nil && strings.TrimSpace(*step.TargetValue) != "" {
			targets = append(targets, strings.TrimSpace(*step.TargetValue))
		} else if queue.EscalationChannelID != nil && strings.TrimSpace(*queue.EscalationChannelID) != "" {
			targets = append(targets, strings.TrimSpace(*queue.EscalationChannelID))
		}
	case "dm_user":
		if step.TargetValue != nil && strings.TrimSpace(*step.TargetValue) != "" {
			targets = append(targets, strings.TrimSpace(*step.TargetValue))
		} else {
			role := "triager"
			if clock.ClockType == "assign" {
				role = "manager"
			}
			if members, err := r.store.ListQueueMembers(ctx, queue.ID); err == nil {
				for _, member := range members {
					if member.Role == role {
						targets = append(targets, strings.TrimSpace(member.UserID))
					}
				}
				if len(targets) == 0 && role != "triager" {
					for _, member := range members {
						if member.Role == "triager" {
							targets = append(targets, strings.TrimSpace(member.UserID))
						}
					}
				}
			}
		}
	}
	if len(targets) == 0 {
		return nil
	}

	out := make([]db.NotificationOutboxItem, 0, len(targets))
	for _, target := range targets {
		payload := mustJSON(map[string]any{
			"request_id":        req.ID.String(),
			"queue_id":          queue.ID.String(),
			"clock_id":          clock.ID.String(),
			"clock_type":        clock.ClockType,
			"step_id":           step.ID.String(),
			"step_target_type":  step.TargetType,
			"step_target_value": target,
			"message_template":  stringOrEmpty(step.MessageTemplate),
		})
		dedupe := fmt.Sprintf("escalation:%s:%s:%s", clock.ID, step.ID, target)
		out = append(out, db.NotificationOutboxItem{
			RequestID:        &req.ID,
			Kind:             "escalation_notification",
			DestinationType:  step.TargetType,
			DestinationValue: optionalStringPtr(target),
			Payload:          payload,
			DedupeKey:        &dedupe,
			Status:           "pending",
			AvailableAt:      now,
		})
	}
	return out
}

func (r *SLARunner) enqueueQueueDigests(ctx context.Context, now time.Time, paidAccessCache map[uuid.UUID]bool) error {
	queues, err := r.store.ListAllQueues(ctx)
	if err != nil {
		return err
	}
	policyCache := map[uuid.UUID]db.QueuePolicy{}
	for _, queue := range queues {
		allowed, err := r.workspaceHasPaidAccess(ctx, queue.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: digest paid access lookup failed workspace=%s: %v", queue.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}
		if queue.EscalationChannelID == nil || strings.TrimSpace(*queue.EscalationChannelID) == "" {
			continue
		}

		policy, ok := policyCache[queue.ID]
		if !ok {
			policy, err = r.store.GetQueuePolicy(ctx, queue.ID)
			if err != nil {
				log.Printf("sla: queue policy lookup failed queue=%s: %v", queue.ID, err)
				continue
			}
			policyCache[queue.ID] = policy
		}

		loc := time.UTC
		if strings.TrimSpace(policy.Timezone) != "" {
			if loaded, err := time.LoadLocation(policy.Timezone); err == nil {
				loc = loaded
			}
		}
		localNow := now.In(loc)
		if policy.DigestWeekday != nil && int(localNow.Weekday()) != *policy.DigestWeekday {
			continue
		}
		digestTime, err := time.ParseInLocation("15:04:05", policy.DigestTime, loc)
		if err != nil {
			digestTime, _ = time.ParseInLocation("15:04:05", "09:00:00", loc)
		}
		scheduled := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), digestTime.Hour(), digestTime.Minute(), digestTime.Second(), 0, loc)
		if localNow.Before(scheduled) {
			continue
		}

		dateKey := localNow.Format("2006-01-02")
		dedupe := fmt.Sprintf("queue_digest:%s:%s", queue.ID, dateKey)
		payload := mustJSON(map[string]any{
			"queue_id": queue.ID.String(),
			"date":     dateKey,
		})
		if _, err := r.store.InsertOutboxItem(ctx, db.NotificationOutboxItem{
			Kind:             "queue_digest",
			DestinationType:  "channel",
			DestinationValue: queue.EscalationChannelID,
			Payload:          payload,
			DedupeKey:        &dedupe,
			Status:           "pending",
			AvailableAt:      now,
		}); err != nil {
			log.Printf("sla: enqueue digest failed queue=%s: %v", queue.ID, err)
		}
	}
	return nil
}

func (r *SLARunner) enqueueManagerWeeklyDigests(ctx context.Context, now time.Time, paidAccessCache map[uuid.UUID]bool) error {
	queues, err := r.store.ListAllQueues(ctx)
	if err != nil {
		return err
	}
	policyCache := map[uuid.UUID]db.QueuePolicy{}
	for _, queue := range queues {
		allowed, err := r.workspaceHasPaidAccess(ctx, queue.WorkspaceID, paidAccessCache)
		if err != nil || !allowed {
			continue
		}
		members, err := r.store.ListQueueMembers(ctx, queue.ID)
		if err != nil {
			log.Printf("sla: weekly digest queue members lookup failed queue=%s: %v", queue.ID, err)
			continue
		}
		managerIDs := make([]string, 0, len(members))
		for _, member := range members {
			if strings.EqualFold(member.Role, "manager") && strings.TrimSpace(member.UserID) != "" {
				managerIDs = append(managerIDs, strings.TrimSpace(member.UserID))
			}
		}
		if len(managerIDs) == 0 {
			continue
		}
		policy, ok := policyCache[queue.ID]
		if !ok {
			policy, err = r.store.GetQueuePolicy(ctx, queue.ID)
			if err != nil {
				log.Printf("sla: weekly digest queue policy lookup failed queue=%s: %v", queue.ID, err)
				continue
			}
			policyCache[queue.ID] = policy
		}
		if policy.DigestWeekday == nil {
			continue
		}
		loc := time.UTC
		if strings.TrimSpace(policy.Timezone) != "" {
			if loaded, err := time.LoadLocation(policy.Timezone); err == nil {
				loc = loaded
			}
		}
		localNow := now.In(loc)
		if int(localNow.Weekday()) != *policy.DigestWeekday {
			continue
		}
		digestTime, err := time.ParseInLocation("15:04:05", policy.DigestTime, loc)
		if err != nil {
			digestTime, _ = time.ParseInLocation("15:04:05", "09:00:00", loc)
		}
		scheduled := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), digestTime.Hour(), digestTime.Minute(), digestTime.Second(), 0, loc)
		if localNow.Before(scheduled) {
			continue
		}
		dateKey := localNow.Format("2006-01-02")
		for _, managerID := range managerIDs {
			dedupe := fmt.Sprintf("manager_weekly_digest:%s:%s:%s", queue.ID, managerID, dateKey)
			payload := mustJSON(map[string]any{
				"queue_id":     queue.ID.String(),
				"manager_id":   managerID,
				"date":         dateKey,
				"workspace_id": queue.WorkspaceID.String(),
			})
			if _, err := r.store.InsertOutboxItem(ctx, db.NotificationOutboxItem{
				Kind:             "manager_weekly_digest",
				DestinationType:  "dm_user",
				DestinationValue: stringPtr(managerID),
				Payload:          payload,
				DedupeKey:        &dedupe,
				Status:           "pending",
				AvailableAt:      now,
			}); err != nil {
				log.Printf("sla: enqueue weekly manager digest failed queue=%s manager=%s: %v", queue.ID, managerID, err)
			}
		}
	}
	return nil
}

func (r *SLARunner) refreshDailyMetrics(ctx context.Context, now time.Time) error {
	queues, err := r.store.ListAllQueues(ctx)
	if err != nil {
		return err
	}
	days := []time.Time{now, now.Add(-24 * time.Hour)}
	for _, queue := range queues {
		for _, day := range days {
			metric, err := r.store.ComputeDailyQueueMetric(ctx, queue, day)
			if err != nil {
				log.Printf("sla: compute daily metric failed queue=%s date=%s: %v", queue.ID, day.Format("2006-01-02"), err)
				continue
			}
			if err := r.store.UpsertDailyQueueMetric(ctx, metric); err != nil {
				log.Printf("sla: upsert daily metric failed queue=%s date=%s: %v", queue.ID, metric.Date, err)
			}
		}
	}
	return nil
}

func mustJSON(payload map[string]any) []byte {
	data, err := json.Marshal(payload)
	if err != nil {
		return []byte(`{}`)
	}
	return data
}

func optionalStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	return &trimmed
}

func stringPtr(value string) *string {
	return &value
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
