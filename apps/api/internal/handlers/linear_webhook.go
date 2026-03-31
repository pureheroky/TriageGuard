package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
)

type linearWebhookEvent struct {
	Action string `json:"action"`
	Type   string `json:"type"`
	Data   struct {
		ID string `json:"id"`
	} `json:"data"`
}

const maxLinearWebhookBodyBytes = 2 << 20 // 2 MiB

func (a *App) handleLinearWebhook(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(a.cfg.LinearWebhookSecret) == "" {
		http.NotFound(w, r)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxLinearWebhookBodyBytes+1))
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(body) > maxLinearWebhookBodyBytes {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := verifyLinearWebhookSignature(body, r.Header.Get("Linear-Signature"), a.cfg.LinearWebhookSecret); err != nil {
		http.Error(w, "invalid linear signature", http.StatusUnauthorized)
		return
	}

	var event linearWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid linear event", http.StatusBadRequest)
		return
	}

	eventAction := strings.ToLower(strings.TrimSpace(event.Action))
	eventType := strings.TrimSpace(event.Type)
	eventName := strings.TrimSpace(eventType + ":" + eventAction)
	deliveryID := linearWebhookDeliveryID(r.Header.Get("Linear-Delivery"), body)

	recorded, err := a.store.TryRecordLinearWebhookEvent(r.Context(), deliveryID, eventName)
	if err != nil {
		a.logger.Printf("linear webhook dedupe insert failed delivery=%s: %v", deliveryID, err)
		http.Error(w, "linear webhook dedupe failed", http.StatusInternalServerError)
		return
	}
	if !recorded {
		jsonResponse(w, http.StatusOK, map[string]any{"received": true, "duplicate": true})
		return
	}

	if !strings.EqualFold(eventType, "Issue") || !isSupportedLinearIssueWebhookAction(eventAction) {
		jsonResponse(w, http.StatusOK, map[string]any{"received": true, "ignored": true})
		return
	}

	issueID := strings.TrimSpace(event.Data.ID)
	if issueID == "" {
		jsonResponse(w, http.StatusOK, map[string]any{"received": true, "ignored": true})
		return
	}

	if eventAction == "remove" {
		a.logger.Printf("linear webhook remove ignored delivery=%s issue_id=%s", deliveryID, issueID)
		jsonResponse(w, http.StatusOK, map[string]any{"received": true, "ignored": true})
		return
	}

	if err := a.syncRequestFromLinearIssue(r.Context(), issueID, deliveryID, eventAction); err != nil {
		var workspaceID *uuid.UUID
		var requestID *uuid.UUID
		if linked, lookupErr := a.store.GetRequestByLinearIssueID(r.Context(), issueID); lookupErr == nil {
			wid := linked.WorkspaceID
			rid := linked.ID
			workspaceID = &wid
			requestID = &rid
		}
		externalID := issueID
		a.recordDeadLetter(r.Context(), "linear_webhook", "sync_request_from_issue", workspaceID, requestID, &externalID, err, map[string]any{
			"delivery_id": deliveryID,
			"event_type":  eventType,
			"action":      eventAction,
		})
		if linear.IsAuthError(err) {
			a.logger.Printf("linear webhook auth warning delivery=%s issue_id=%s: %v", deliveryID, issueID, err)
			jsonResponse(w, http.StatusOK, map[string]any{"received": true, "warning": "linear_auth_required"})
			return
		}
		a.logger.Printf("linear webhook sync failed delivery=%s issue_id=%s: %v", deliveryID, issueID, err)
		http.Error(w, "linear webhook sync failed", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, http.StatusOK, map[string]any{"received": true})
}

func (a *App) syncRequestFromLinearIssue(ctx context.Context, issueID, deliveryID, action string) error {
	request, err := a.store.GetRequestByLinearIssueID(ctx, issueID)
	if err != nil {
		if errorsIsNoRows(err) {
			a.logger.Printf("linear webhook ignored delivery=%s issue_id=%s reason=unlinked_issue", deliveryID, issueID)
			return nil
		}
		return err
	}

	policy, err := a.store.GetPolicy(ctx, request.WorkspaceID)
	if err != nil {
		return err
	}

	install, installErr := a.store.GetSlackInstallationByWorkspace(ctx, request.WorkspaceID)
	hasSlackInstall := installErr == nil
	if installErr != nil && !errorsIsNoRows(installErr) {
		return installErr
	}

	var snapshot linear.IssueSnapshot
	if err := a.withLinearAccessToken(ctx, request.WorkspaceID, func(token string) error {
		issue, err := a.linearClient.GetIssue(ctx, token, issueID)
		if err != nil {
			return err
		}
		snapshot = issue
		return nil
	}); err != nil {
		return err
	}

	if strings.TrimSpace(snapshot.URL) != "" {
		_ = a.store.UpsertLinearIssue(ctx, db.LinearIssue{RequestID: request.ID, IssueID: issueID, IssueURL: snapshot.URL})
	}

	statusSynced := false
	switch requestStatusSyncActionFromLinearState(request.Status, snapshot.StateType) {
	case "SYNC_RESOLVE":
		if err := a.store.SetResolve(ctx, request.ID); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{
			RequestID:    request.ID,
			ActorSlackID: linearWebhookActorID,
			ActionType:   "SYNC_RESOLVE",
			Payload: marshalSyncPayload(map[string]any{
				"delivery_id":       deliveryID,
				"linear_issue_id":   issueID,
				"linear_state_type": snapshot.StateType,
			}),
		})
		statusSynced = true
	case "SYNC_IGNORE":
		if err := a.store.SetIgnore(ctx, request.ID); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{
			RequestID:    request.ID,
			ActorSlackID: linearWebhookActorID,
			ActionType:   "SYNC_IGNORE",
			Payload: marshalSyncPayload(map[string]any{
				"delivery_id":       deliveryID,
				"linear_issue_id":   issueID,
				"linear_state_type": snapshot.StateType,
			}),
		})
		statusSynced = true
	case "SYNC_REOPEN":
		if err := a.store.SetReopen(ctx, request.ID); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{
			RequestID:    request.ID,
			ActorSlackID: linearWebhookActorID,
			ActionType:   "SYNC_REOPEN",
			Payload: marshalSyncPayload(map[string]any{
				"delivery_id":       deliveryID,
				"linear_issue_id":   issueID,
				"linear_state_type": snapshot.StateType,
			}),
		})
		statusSynced = true
	}

	if refreshed, err := a.store.GetRequestByID(ctx, request.ID); err == nil {
		request = refreshed
	}

	dueSynced := false
	if !sameDueDate(request.DueAt, snapshot.DueDate, policy.Timezone) {
		dueAt, err := linearDueDateToDueAt(snapshot.DueDate, policy.Timezone)
		if err != nil {
			return err
		}
		if err := a.store.SetDue(ctx, request.ID, dueAt); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{
			RequestID:    request.ID,
			ActorSlackID: linearWebhookActorID,
			ActionType:   "SYNC_DUE",
			Payload: marshalSyncPayload(map[string]any{
				"delivery_id":     deliveryID,
				"linear_issue_id": issueID,
				"due_date":        snapshot.DueDate,
			}),
		})
		dueSynced = true
	}

	if refreshed, err := a.store.GetRequestByID(ctx, request.ID); err == nil {
		request = refreshed
	}

	assigneeSynced := false
	assigneeSkipped := false
	var targetOwnerSlackID *string
	assigneeEmail := ""
	if snapshot.AssigneeEmail != nil {
		assigneeEmail = strings.TrimSpace(*snapshot.AssigneeEmail)
	}
	if assigneeEmail == "" {
		targetOwnerSlackID = nil
	} else if !hasSlackInstall {
		assigneeSkipped = true
	} else {
		mappedSlackUserID, err := a.slackClient.LookupUserByEmail(ctx, install.BotToken, assigneeEmail)
		if err != nil {
			return err
		}
		mappedSlackUserID = strings.TrimSpace(mappedSlackUserID)
		if mappedSlackUserID == "" {
			assigneeSkipped = true
			a.logger.Printf("linear webhook assignee not matched delivery=%s issue_id=%s email=%s", deliveryID, issueID, assigneeEmail)
		} else {
			targetOwnerSlackID = &mappedSlackUserID
		}
	}

	if !assigneeSkipped && !sameOptionalString(request.OwnerSlackID, targetOwnerSlackID) {
		if err := a.store.UpdateOwnerFromExternal(ctx, request.ID, targetOwnerSlackID); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{
			RequestID:    request.ID,
			ActorSlackID: linearWebhookActorID,
			ActionType:   "SYNC_ASSIGN",
			Payload: marshalSyncPayload(map[string]any{
				"delivery_id":      deliveryID,
				"linear_issue_id":  issueID,
				"assignee_email":   assigneeEmail,
				"owner_slack_id":   targetOwnerSlackID,
				"assignee_skipped": false,
			}),
		})
		assigneeSynced = true
	}

	if hasSlackInstall {
		if err := a.refreshTriageCard(ctx, install, request.ID); err != nil {
			return err
		}
	}

	a.logger.Printf(
		"linear webhook synced delivery=%s action=%s issue_id=%s request_id=%s status_synced=%t due_synced=%t assignee_synced=%t assignee_skipped=%t",
		deliveryID,
		action,
		issueID,
		request.ID,
		statusSynced,
		dueSynced,
		assigneeSynced,
		assigneeSkipped,
	)

	return nil
}

func verifyLinearWebhookSignature(body []byte, headerValue, webhookSecret string) error {
	secret := strings.TrimSpace(webhookSecret)
	if secret == "" {
		return errors.New("linear webhook secret is required")
	}
	signatures := parseLinearSignatureCandidates(headerValue)
	if len(signatures) == 0 {
		return errors.New("missing linear signature")
	}

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	digest := mac.Sum(nil)
	expectedHex := hex.EncodeToString(digest)
	expectedBase64 := base64.StdEncoding.EncodeToString(digest)

	for _, sig := range signatures {
		normalized := strings.ToLower(strings.TrimSpace(sig))
		if normalized == "" {
			continue
		}
		if after, ok := strings.CutPrefix(normalized, "sha256="); ok  {
			normalized = after
		}
		if hmac.Equal([]byte(normalized), []byte(strings.ToLower(expectedHex))) {
			return nil
		}
		if hmac.Equal([]byte(strings.TrimSpace(sig)), []byte(expectedBase64)) {
			return nil
		}
	}

	return errors.New("linear signature mismatch")
}

func parseLinearSignatureCandidates(headerValue string) []string {
	raw := strings.TrimSpace(headerValue)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "=") {
			kv := strings.SplitN(trimmed, "=", 2)
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			value := strings.TrimSpace(kv[1])
			if key == "v1" || key == "sha256" || key == "signature" {
				out = append(out, value)
			}
			continue
		}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		out = append(out, raw)
	}
	return out
}

func linearWebhookDeliveryID(deliveryHeader string, body []byte) string {
	deliveryID := strings.TrimSpace(deliveryHeader)
	if deliveryID != "" {
		return deliveryID
	}
	sum := sha256.Sum256(body)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func isSupportedLinearIssueWebhookAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "create", "update", "remove":
		return true
	default:
		return false
	}
}
