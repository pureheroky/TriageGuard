package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
)

const linearWebhookActorID = "LINEAR_WEBHOOK"

func (a *App) trySyncLinkedLinearIssueFromSlack(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID, slackUserID string) {
	channelID := ""
	var workspaceID *uuid.UUID
	if request, err := a.store.GetRequestByID(ctx, requestID); err == nil {
		channelID = strings.TrimSpace(request.ChannelID)
		id := request.WorkspaceID
		workspaceID = &id
	}
	a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, workspaceID, channelID, slackUserID)
}

func (a *App) trySyncLinkedLinearIssueFromSlackInChannel(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID, workspaceID *uuid.UUID, channelID, slackUserID string) {
	if err := a.syncLinkedLinearIssueFromSlack(ctx, install, requestID); err != nil {
		a.logger.Printf("linear sync from slack failed request_id=%s: %v", requestID, err)
		requestIDCopy := requestID
		a.recordDeadLetter(ctx, "slack_to_linear_sync", "update_linked_issue", workspaceID, &requestIDCopy, nil, err, map[string]any{
			"channel_id":    strings.TrimSpace(channelID),
			"slack_user_id": strings.TrimSpace(slackUserID),
		})
		if strings.TrimSpace(slackUserID) == "" || strings.TrimSpace(channelID) == "" {
			return
		}
		message := "Request updated in Slack, but sync to Linear failed."
		lower := strings.ToLower(err.Error())
		switch {
		case linear.IsAuthError(err), strings.Contains(lower, "linear auth required"), strings.Contains(lower, "not authenticated"):
			message = "Request updated in Slack, but Linear session is expired/revoked. Reconnect Linear in Admin Console."
		case strings.Contains(lower, "team"), strings.Contains(lower, "state"):
			message = "Request updated in Slack, but Linear state sync failed. Check selected Linear team/workflow."
		case strings.Contains(lower, "assignee"):
			message = "Request updated in Slack, but assignee sync to Linear failed."
		}
		if err := a.slackClient.PostEphemeral(ctx, install.BotToken, strings.TrimSpace(channelID), slackUserID, message); err != nil {
			a.logger.Printf("linear sync warning notify failed request_id=%s user=%s: %v", requestID, slackUserID, err)
		}
	}
}

func (a *App) syncLinkedLinearIssueFromSlack(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID) error {
	linkedIssue, err := a.store.GetLinearIssueByRequest(ctx, requestID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}

	request, err := a.store.GetRequestByID(ctx, requestID)
	if err != nil {
		return err
	}
	policy, err := a.store.GetPolicy(ctx, request.WorkspaceID)
	if err != nil {
		return err
	}

	var snapshot linear.IssueSnapshot
	if err := a.withLinearAccessToken(ctx, request.WorkspaceID, func(token string) error {
		issue, err := a.linearClient.GetIssue(ctx, token, linkedIssue.IssueID)
		if err != nil {
			return err
		}
		snapshot = issue
		return nil
	}); err != nil {
		return err
	}

	teamID := strings.TrimSpace(snapshot.TeamID)
	if policy.LinearTeamID != nil && strings.TrimSpace(*policy.LinearTeamID) != "" {
		teamID = strings.TrimSpace(*policy.LinearTeamID)
	}

	var stateID *string
	setState := false
	desiredStateType := linearStateTypeForRequestStatus(request.Status)
	if teamID != "" && desiredStateType != "" {
		if strings.EqualFold(strings.TrimSpace(snapshot.StateType), strings.TrimSpace(desiredStateType)) {
			desiredStateType = ""
		}
	}
	if teamID != "" && desiredStateType != "" {
		var states []linear.WorkflowState
		if err := a.withLinearAccessToken(ctx, request.WorkspaceID, func(token string) error {
			fetched, err := a.linearClient.ListTeamStates(ctx, token, teamID)
			if err != nil {
				return err
			}
			states = fetched
			return nil
		}); err != nil {
			return err
		}
		if selectedStateID := pickLinearStateID(states, desiredStateType); selectedStateID != "" {
			stateID = &selectedStateID
			setState = true
		}
	}

	var dueDate *string
	setDueDate := false
	if request.DueAt != nil {
		dueDate = dueAtToLinearDate(request.DueAt, policy.Timezone)
	}
	if !sameDueDate(request.DueAt, snapshot.DueDate, policy.Timezone) {
		setDueDate = true
	}

	var assigneeID *string
	setAssignee := false
	if request.OwnerSlackID == nil || strings.TrimSpace(*request.OwnerSlackID) == "" {
		setAssignee = snapshot.AssigneeID != nil && strings.TrimSpace(*snapshot.AssigneeID) != ""
	} else {
		email, err := a.slackClient.GetUserEmail(ctx, install.BotToken, strings.TrimSpace(*request.OwnerSlackID))
		if err != nil {
			a.logger.Printf("linear sync from slack: resolve owner email failed request_id=%s owner=%s: %v", request.ID, strings.TrimSpace(*request.OwnerSlackID), err)
		} else if strings.TrimSpace(email) != "" {
			var resolvedAssigneeID string
			if err := a.withLinearAccessToken(ctx, request.WorkspaceID, func(token string) error {
				userID, err := a.linearClient.FindUserIDByEmail(ctx, token, email)
				if err != nil {
					return err
				}
				resolvedAssigneeID = strings.TrimSpace(userID)
				return nil
			}); err != nil {
				return err
			}
			if resolvedAssigneeID != "" {
				assigneeID = &resolvedAssigneeID
				if snapshot.AssigneeID == nil || strings.TrimSpace(*snapshot.AssigneeID) != resolvedAssigneeID {
					setAssignee = true
				}
			} else {
				a.logger.Printf("linear sync from slack: owner email has no linear match request_id=%s email=%s", request.ID, email)
			}
		}
	}

	if !setState && !setDueDate && !setAssignee {
		return nil
	}

	return a.withLinearAccessToken(ctx, request.WorkspaceID, func(token string) error {
		return a.linearClient.UpdateIssue(ctx, token, linear.UpdateIssueInput{
			IssueID:     linkedIssue.IssueID,
			AssigneeID:  assigneeID,
			DueDate:     dueDate,
			StateID:     stateID,
			SetAssignee: setAssignee,
			SetDueDate:  setDueDate,
			SetState:    setState,
		})
	})
}

func requestStatusSyncActionFromLinearState(currentStatus, linearStateType string) string {
	normalizedState := strings.ToLower(strings.TrimSpace(linearStateType))
	normalizedStatus := strings.ToUpper(strings.TrimSpace(currentStatus))

	switch normalizedState {
	case "completed":
		if normalizedStatus == "IGNORED" {
			return ""
		}
		if normalizedStatus != "RESOLVED" {
			return "SYNC_RESOLVE"
		}
		return ""
	case "canceled":
		if normalizedStatus == "RESOLVED" {
			return ""
		}
		if normalizedStatus != "IGNORED" {
			return "SYNC_IGNORE"
		}
		return ""
	case "unstarted", "started", "backlog", "triage":
		if normalizedStatus == "RESOLVED" || normalizedStatus == "IGNORED" {
			return "SYNC_REOPEN"
		}
		return ""
	default:
		return ""
	}
}

func dueAtToLinearDate(dueAt *time.Time, timezone string) *string {
	if dueAt == nil {
		return nil
	}
	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(timezone)); err == nil {
			loc = parsed
		}
	}
	v := dueAt.In(loc).Format("2006-01-02")
	return &v
}

func linearDueDateToDueAt(rawDueDate *string, timezone string) (*time.Time, error) {
	if rawDueDate == nil || strings.TrimSpace(*rawDueDate) == "" {
		return nil, nil
	}

	loc := time.UTC
	if strings.TrimSpace(timezone) != "" {
		if parsed, err := time.LoadLocation(strings.TrimSpace(timezone)); err == nil {
			loc = parsed
		}
	}

	datePart := strings.TrimSpace(*rawDueDate)
	t, err := time.ParseInLocation("2006-01-02", datePart, loc)
	if err != nil {
		return nil, err
	}
	eod := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc).UTC()
	return &eod, nil
}

func sameDueDate(existing *time.Time, incoming *string, timezone string) bool {
	existingDate := ""
	if existing != nil {
		if d := dueAtToLinearDate(existing, timezone); d != nil {
			existingDate = strings.TrimSpace(*d)
		}
	}
	incomingDate := ""
	if incoming != nil {
		incomingDate = strings.TrimSpace(*incoming)
	}
	return existingDate == incomingDate
}

func sameOptionalString(a, b *string) bool {
	left := ""
	right := ""
	if a != nil {
		left = strings.TrimSpace(*a)
	}
	if b != nil {
		right = strings.TrimSpace(*b)
	}
	return left == right
}

func marshalSyncPayload(data map[string]any) json.RawMessage {
	payload, err := json.Marshal(data)
	if err != nil {
		fallback, _ := json.Marshal(map[string]any{"error": fmt.Sprintf("payload_marshal_failed: %v", err)})
		return fallback
	}
	return payload
}
