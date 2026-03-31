package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
	slackapi "triageguard/apps/api/internal/slack"
)

const notificationDispatcherLockKey int64 = 81273452
const maxOutboxRetries = 6

type NotificationDispatcher struct {
	cfg          config.Config
	store        *db.Store
	slackClient  *slackapi.Client
	linearClient *linear.Client
}

type terminalOutboxError struct {
	err error
}

func (e terminalOutboxError) Error() string {
	return e.err.Error()
}

func (e terminalOutboxError) Unwrap() error {
	return e.err
}

func NewNotificationDispatcher(cfg config.Config, store *db.Store, slackClient *slackapi.Client, linearClient *linear.Client) *NotificationDispatcher {
	return &NotificationDispatcher{cfg: cfg, store: store, slackClient: slackClient, linearClient: linearClient}
}

func (d *NotificationDispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if err := d.RunOnce(ctx); err != nil {
		log.Printf("outbox: initial run failed: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.RunOnce(ctx); err != nil {
				log.Printf("outbox: periodic run failed: %v", err)
			}
		}
	}
}

func (d *NotificationDispatcher) RunOnce(ctx context.Context) error {
	startedAt := time.Now().UTC()
	_ = d.store.RecordJobRuntimeStart(ctx, "notification_dispatcher", startedAt)
	var runErr error
	defer func() {
		_ = d.store.RecordJobRuntimeFinish(context.Background(), "notification_dispatcher", time.Now().UTC(), time.Since(startedAt), runErr)
	}()

	locked, err := d.store.TryAdvisoryLock(ctx, notificationDispatcherLockKey)
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
		if err := d.store.AdvisoryUnlock(unlockCtx, notificationDispatcherLockKey); err != nil {
			log.Printf("outbox: advisory unlock failed: %v", err)
		}
	}()

	for {
		items, err := d.store.ClaimNotificationOutbox(ctx, 25)
		if err != nil {
			runErr = err
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for _, item := range items {
			if err := d.processItem(ctx, item); err != nil {
				terminal := isTerminalOutboxError(err)
				delay := retryDelay(item.RetryCount + 1)
				if markErr := d.store.MarkNotificationOutboxRetry(ctx, item.ID, err.Error(), time.Now().UTC().Add(delay), terminal || item.RetryCount+1 >= maxOutboxRetries); markErr != nil {
					log.Printf("outbox: mark retry failed item=%s: %v", item.ID, markErr)
				}
				if terminal || item.RetryCount+1 >= maxOutboxRetries {
					d.recordDeadLetter(ctx, item, err)
				}
				continue
			}
			if err := d.store.MarkNotificationOutboxSent(ctx, item.ID); err != nil {
				log.Printf("outbox: mark sent failed item=%s: %v", item.ID, err)
			}
		}
		if len(items) < 25 {
			return nil
		}
	}
}

func (d *NotificationDispatcher) processItem(ctx context.Context, item db.NotificationOutboxItem) error {
	switch item.Kind {
	case "triage_card_refresh":
		return d.processTriageCardRefresh(ctx, item)
	case "external_sync_request":
		return d.processExternalSync(ctx, item)
	case "tracker_link_request":
		return d.processTrackerLink(ctx, item)
	case "escalation_notification":
		return d.processEscalation(ctx, item)
	case "queue_digest":
		return d.processQueueDigest(ctx, item)
	case "manager_weekly_digest":
		return d.processManagerWeeklyDigest(ctx, item)
	default:
		return terminalOutboxError{err: fmt.Errorf("unknown outbox kind: %s", item.Kind)}
	}
}

func (d *NotificationDispatcher) processTriageCardRefresh(ctx context.Context, item db.NotificationOutboxItem) error {
	requestID, err := d.outboxRequestID(item)
	if err != nil {
		return terminalOutboxError{err: err}
	}
	req, err := d.store.GetRequestByID(ctx, requestID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	install, err := d.store.GetSlackInstallationByWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	return d.refreshTriageCard(ctx, install, req.ID)
}

func (d *NotificationDispatcher) processEscalation(ctx context.Context, item db.NotificationOutboxItem) error {
	requestID, err := d.outboxRequestID(item)
	if err != nil {
		return terminalOutboxError{err: err}
	}
	req, err := d.store.GetRequestByID(ctx, requestID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	install, err := d.store.GetSlackInstallationByWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}

	var payload struct {
		ClockID         string `json:"clock_id"`
		ClockType       string `json:"clock_type"`
		StepID          string `json:"step_id"`
		StepTargetType  string `json:"step_target_type"`
		StepTargetValue string `json:"step_target_value"`
		MessageTemplate string `json:"message_template"`
	}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return terminalOutboxError{err: err}
	}

	clocks, _ := d.store.ListRequestSLAClocks(ctx, req.ID)
	slaHint := slackapi.RenderSLAHint(req, clocks, time.Now().UTC())
	threadURL := buildThreadURL(install.TeamDomain, req.ChannelID, req.ThreadTS)
	message := strings.TrimSpace(payload.MessageTemplate)
	if message == "" {
		switch payload.ClockType {
		case "ack":
			message = "Ack overdue"
		case "assign":
			message = "Assign overdue"
		case "stale":
			message = "Request is stale"
		default:
			message = "Request needs attention"
		}
	}
	if payload.StepTargetType == "thread" {
		if _, err := d.slackClient.PostMessage(ctx, install.BotToken, req.ChannelID, "⚠️ "+message, req.ThreadTS, nil); err != nil {
			return err
		}
	} else {
		target := payload.StepTargetValue
		if strings.TrimSpace(target) == "" && item.DestinationValue != nil {
			target = strings.TrimSpace(*item.DestinationValue)
		}
		if strings.TrimSpace(target) == "" {
			return nil
		}
		text := "⚠️ " + message
		if threadURL != "" {
			text = fmt.Sprintf("%s: <%s|open thread>", text, threadURL)
		}
		if slaHint != "" {
			text += "\n" + strings.ReplaceAll(slaHint, "*", "")
		}
		if _, err := d.slackClient.PostMessage(ctx, install.BotToken, target, text, "", nil); err != nil {
			return err
		}
	}

	if payload.ClockID != "" {
		if clockID, err := uuid.Parse(payload.ClockID); err == nil {
			_ = d.store.SetClockLastNotified(ctx, clockID, time.Now().UTC())
		}
	}
	eventPayload := mustJSON(map[string]any{
		"clock_type":   payload.ClockType,
		"step_id":      payload.StepID,
		"target_type":  payload.StepTargetType,
		"target_value": payload.StepTargetValue,
		"thread_url":   threadURL,
	})
	_, _ = d.store.InsertRequestEvent(ctx, db.RequestEvent{
		RequestID:  req.ID,
		EventType:  "escalation_sent",
		ActorType:  "scheduler",
		OccurredAt: time.Now().UTC(),
		Payload:    eventPayload,
	})
	return nil
}

func (d *NotificationDispatcher) processQueueDigest(ctx context.Context, item db.NotificationOutboxItem) error {
	var payload struct {
		QueueID string `json:"queue_id"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return terminalOutboxError{err: err}
	}
	queueID, err := uuid.Parse(strings.TrimSpace(payload.QueueID))
	if err != nil {
		return terminalOutboxError{err: err}
	}
	queue, err := d.store.GetQueueByID(ctx, queueID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	if item.DestinationValue == nil || strings.TrimSpace(*item.DestinationValue) == "" {
		if queue.EscalationChannelID == nil || strings.TrimSpace(*queue.EscalationChannelID) == "" {
			return nil
		}
		item.DestinationValue = queue.EscalationChannelID
	}
	install, err := d.store.GetSlackInstallationByWorkspace(ctx, queue.WorkspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	detail, err := d.store.GetQueueDetailMetrics(ctx, queue.ID)
	if err != nil {
		return err
	}
	_, queueRows, _, _, err := d.store.QueueDashboard(ctx, queue.WorkspaceID)
	if err != nil {
		return err
	}
	openCount := 0
	for _, row := range queueRows {
		if row.Queue.ID == queue.ID {
			openCount = row.OpenCount
			break
		}
	}
	topRequests, err := d.store.ListQueueOpenRequests(ctx, queue.ID, 5)
	if err != nil {
		return err
	}
	lines := []string{
		fmt.Sprintf("*Queue Digest: %s*", queue.Name),
		fmt.Sprintf("- Open: %d", openCount),
		fmt.Sprintf("- Unacked: %d", detail.UnackedCount),
		fmt.Sprintf("- Unassigned: %d", detail.UnassignedCount),
		fmt.Sprintf("- Waiting: %d", detail.WaitingCount),
		fmt.Sprintf("- Breached: %d", detail.BreachedCount),
		fmt.Sprintf("- At risk: %d", detail.AtRiskCount),
	}
	if len(topRequests) > 0 {
		lines = append(lines, "- Open requests:")
		for _, req := range topRequests {
			label := req.Priority
			if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
				label += " - " + strings.TrimSpace(*req.Title)
			}
			threadURL := buildThreadURL(install.TeamDomain, req.ChannelID, req.ThreadTS)
			if threadURL != "" {
				lines = append(lines, fmt.Sprintf("  - <%s|%s>", threadURL, label))
			} else {
				lines = append(lines, fmt.Sprintf("  - %s", label))
			}
		}
	}
	_, err = d.slackClient.PostMessage(ctx, install.BotToken, *item.DestinationValue, strings.Join(lines, "\n"), "", nil)
	return err
}

func (d *NotificationDispatcher) processManagerWeeklyDigest(ctx context.Context, item db.NotificationOutboxItem) error {
	var payload struct {
		QueueID   string `json:"queue_id"`
		ManagerID string `json:"manager_id"`
		Date      string `json:"date"`
	}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return terminalOutboxError{err: err}
	}
	queueID, err := uuid.Parse(strings.TrimSpace(payload.QueueID))
	if err != nil {
		return terminalOutboxError{err: err}
	}
	queue, err := d.store.GetQueueByID(ctx, queueID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	install, err := d.store.GetSlackInstallationByWorkspace(ctx, queue.WorkspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	detail, err := d.store.GetQueueDetailMetrics(ctx, queue.ID)
	if err != nil {
		return err
	}
	_, queueRows, _, _, err := d.store.QueueDashboard(ctx, queue.WorkspaceID)
	if err != nil {
		return err
	}
	openCount := 0
	for _, row := range queueRows {
		if row.Queue.ID == queue.ID {
			openCount = row.OpenCount
			break
		}
	}
	metrics, err := d.store.ListRecentDailyQueueMetrics(ctx, queue.ID, 14)
	if err != nil {
		return err
	}
	currentBreached := 0
	previousBreached := 0
	currentReopens := 0
	previousReopens := 0
	currentUnassigned := 0
	previousUnassigned := 0
	split := len(metrics) / 2
	for idx, metric := range metrics {
		breached := metric.BreachedAckCount + metric.BreachedAssignCount
		if idx < split {
			previousBreached += breached
			previousReopens += metric.ReopenCount
			previousUnassigned += metric.UnassignedCount
		} else {
			currentBreached += breached
			currentReopens += metric.ReopenCount
			currentUnassigned += metric.UnassignedCount
		}
	}
	lines := []string{
		fmt.Sprintf("*Weekly Queue Health: %s*", queue.Name),
		fmt.Sprintf("- Open: %d", openCount),
		fmt.Sprintf("- Unacked: %d", detail.UnackedCount),
		fmt.Sprintf("- Unassigned debt: %d", detail.UnassignedCount),
		fmt.Sprintf("- Waiting debt: %d", detail.WaitingDebtCount),
		fmt.Sprintf("- No human activity: %d", detail.NoHumanActivityCount),
	}
	if currentBreached > previousBreached {
		lines = append(lines, fmt.Sprintf("What worsened: breached SLA volume increased from %d to %d.", previousBreached, currentBreached))
	}
	if currentUnassigned > previousUnassigned {
		lines = append(lines, fmt.Sprintf("What worsened: unassigned debt increased from %d to %d.", previousUnassigned, currentUnassigned))
	}
	if currentReopens > previousReopens {
		lines = append(lines, fmt.Sprintf("What worsened: reopen count increased from %d to %d.", previousReopens, currentReopens))
	}
	actionItems := make([]string, 0, 3)
	if detail.UnackedCount > 0 {
		actionItems = append(actionItems, fmt.Sprintf("ack %d unacked requests", detail.UnackedCount))
	}
	if detail.UnassignedCount > 0 {
		actionItems = append(actionItems, fmt.Sprintf("assign owners to %d requests", detail.UnassignedCount))
	}
	if detail.NoHumanActivityCount > 0 {
		actionItems = append(actionItems, fmt.Sprintf("review %d requests with no human activity", detail.NoHumanActivityCount))
	}
	if len(actionItems) > 0 {
		lines = append(lines, "This week:")
		for _, item := range actionItems {
			lines = append(lines, "- "+item)
		}
	} else {
		lines = append(lines, "This week: keep queue health steady and review at-risk requests before they breach.")
	}
	target := strings.TrimSpace(payload.ManagerID)
	if target == "" && item.DestinationValue != nil {
		target = strings.TrimSpace(*item.DestinationValue)
	}
	if target == "" {
		return nil
	}
	_, err = d.slackClient.PostMessage(ctx, install.BotToken, target, strings.Join(lines, "\n"), "", nil)
	return err
}

func (d *NotificationDispatcher) processExternalSync(ctx context.Context, item db.NotificationOutboxItem) error {
	requestID, err := d.outboxRequestID(item)
	if err != nil {
		return terminalOutboxError{err: err}
	}
	req, err := d.store.GetRequestByID(ctx, requestID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	linkedIssue, err := d.store.GetExternalIssueByProvider(ctx, req.ID, "linear")
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(linkedIssue.ExternalID) == "" {
		return nil
	}

	policy, err := d.store.GetPolicy(ctx, req.WorkspaceID)
	if err != nil {
		return err
	}
	install, installErr := d.store.GetSlackInstallationByWorkspace(ctx, req.WorkspaceID)
	hasSlackInstall := installErr == nil
	if installErr != nil && !errorsIsNoRows(installErr) {
		return installErr
	}

	var snapshot linear.IssueSnapshot
	if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
		issue, err := d.linearClient.GetIssue(ctx, token, linkedIssue.ExternalID)
		if err != nil {
			return err
		}
		snapshot = issue
		return nil
	}); err != nil {
		return classifyLinearError(err)
	}

	teamID := strings.TrimSpace(snapshot.TeamID)
	if policy.LinearTeamID != nil && strings.TrimSpace(*policy.LinearTeamID) != "" {
		teamID = strings.TrimSpace(*policy.LinearTeamID)
	}

	var stateID *string
	setState := false
	desiredStateType := linearStateTypeForRequestStatus(req.Status)
	if teamID != "" && desiredStateType != "" {
		if strings.EqualFold(strings.TrimSpace(snapshot.StateType), strings.TrimSpace(desiredStateType)) {
			desiredStateType = ""
		}
	}
	if teamID != "" && desiredStateType != "" {
		var states []linear.WorkflowState
		if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
			fetched, err := d.linearClient.ListTeamStates(ctx, token, teamID)
			if err != nil {
				return err
			}
			states = fetched
			return nil
		}); err != nil {
			return classifyLinearError(err)
		}
		if selectedStateID := pickLinearStateID(states, desiredStateType); selectedStateID != "" {
			stateID = &selectedStateID
			setState = true
		}
	}

	var dueDate *string
	setDueDate := false
	if req.DueAt != nil {
		dueDate = dueAtToLinearDate(req.DueAt, policy.Timezone)
	}
	if !sameDueDate(req.DueAt, snapshot.DueDate, policy.Timezone) {
		setDueDate = true
	}

	var assigneeID *string
	setAssignee := false
	if req.OwnerSlackID == nil || strings.TrimSpace(*req.OwnerSlackID) == "" {
		setAssignee = snapshot.AssigneeID != nil && strings.TrimSpace(*snapshot.AssigneeID) != ""
	} else if hasSlackInstall {
		email, err := d.slackClient.GetUserEmail(ctx, install.BotToken, strings.TrimSpace(*req.OwnerSlackID))
		if err == nil && strings.TrimSpace(email) != "" {
			var resolvedAssigneeID string
			if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
				userID, err := d.linearClient.FindUserIDByEmail(ctx, token, email)
				if err != nil {
					return err
				}
				resolvedAssigneeID = strings.TrimSpace(userID)
				return nil
			}); err != nil {
				return classifyLinearError(err)
			}
			if resolvedAssigneeID != "" {
				assigneeID = &resolvedAssigneeID
				if snapshot.AssigneeID == nil || strings.TrimSpace(*snapshot.AssigneeID) != resolvedAssigneeID {
					setAssignee = true
				}
			}
		}
	}

	if setState || setDueDate || setAssignee {
		if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
			return d.linearClient.UpdateIssue(ctx, token, linear.UpdateIssueInput{
				IssueID:     linkedIssue.ExternalID,
				AssigneeID:  assigneeID,
				DueDate:     dueDate,
				StateID:     stateID,
				SetAssignee: setAssignee,
				SetDueDate:  setDueDate,
				SetState:    setState,
			})
		}); err != nil {
			lastErr := err.Error()
			now := time.Now().UTC()
			_, _ = d.store.UpsertExternalIssue(ctx, db.ExternalIssue{
				RequestID:     req.ID,
				Provider:      "linear",
				ExternalID:    linkedIssue.ExternalID,
				ExternalKey:   linkedIssue.ExternalKey,
				ExternalURL:   linkedIssue.ExternalURL,
				SyncState:     "failed",
				LastSyncAt:    &now,
				LastSyncError: &lastErr,
			})
			return classifyLinearError(err)
		}
	}

	now := time.Now().UTC()
	_, _ = d.store.UpsertExternalIssue(ctx, db.ExternalIssue{
		RequestID:     req.ID,
		Provider:      "linear",
		ExternalID:    linkedIssue.ExternalID,
		ExternalKey:   linkedIssue.ExternalKey,
		ExternalURL:   linkedIssue.ExternalURL,
		SyncState:     "linked",
		LastSyncAt:    &now,
		LastSyncError: nil,
	})
	_, _ = d.store.InsertRequestEvent(ctx, db.RequestEvent{
		RequestID:  req.ID,
		EventType:  "external_sync_succeeded",
		ActorType:  "scheduler",
		OccurredAt: now,
		Payload:    mustJSON(map[string]any{"provider": "linear", "external_id": linkedIssue.ExternalID}),
	})
	return nil
}

func (d *NotificationDispatcher) processTrackerLink(ctx context.Context, item db.NotificationOutboxItem) error {
	requestID, err := d.outboxRequestID(item)
	if err != nil {
		return terminalOutboxError{err: err}
	}
	req, err := d.store.GetRequestByID(ctx, requestID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}
	if req.State == db.RequestStateClosed {
		return nil
	}
	if existing, err := d.store.GetExternalIssueByProvider(ctx, requestID, "linear"); err == nil && strings.TrimSpace(existing.ExternalID) != "" {
		return d.processTriageCardRefresh(ctx, item)
	} else if err != nil && !errorsIsNoRows(err) {
		return err
	}

	install, err := d.store.GetSlackInstallationByWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		if errorsIsNoRows(err) {
			return terminalOutboxError{err: fmt.Errorf("slack is not connected")}
		}
		return err
	}
	if _, err := d.store.GetLinearInstallation(ctx, req.WorkspaceID); err != nil {
		if errorsIsNoRows(err) {
			return terminalOutboxError{err: fmt.Errorf("linear is not connected")}
		}
		return err
	}
	policy, err := d.store.GetPolicy(ctx, req.WorkspaceID)
	if err != nil {
		return err
	}
	if policy.LinearTeamID == nil || strings.TrimSpace(*policy.LinearTeamID) == "" {
		return terminalOutboxError{err: fmt.Errorf("linear team is not configured")}
	}

	threadURL := buildThreadURL(install.TeamDomain, req.ChannelID, req.ThreadTS)
	title := "Triage: " + strings.TrimSpace(req.BodyText)
	if req.Title != nil && strings.TrimSpace(*req.Title) != "" {
		title = "Triage: " + strings.TrimSpace(*req.Title)
	}

	loc := time.UTC
	if strings.TrimSpace(policy.Timezone) != "" {
		if loaded, err := time.LoadLocation(policy.Timezone); err == nil {
			loc = loaded
		}
	}

	ownerDisplay := "Unassigned"
	var assigneeID *string
	if req.OwnerUserID != nil && strings.TrimSpace(*req.OwnerUserID) != "" {
		ownerID := strings.TrimSpace(*req.OwnerUserID)
		ownerDisplay = ownerID
		if displayName, err := d.slackClient.GetUserDisplayName(ctx, install.BotToken, ownerID); err == nil && strings.TrimSpace(displayName) != "" {
			ownerDisplay = strings.TrimSpace(displayName)
		}
		if email, err := d.slackClient.GetUserEmail(ctx, install.BotToken, ownerID); err == nil && strings.TrimSpace(email) != "" {
			var linearUserID string
			if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
				resolvedID, err := d.linearClient.FindUserIDByEmail(ctx, token, email)
				if err != nil {
					return err
				}
				linearUserID = resolvedID
				return nil
			}); err != nil {
				return classifyLinearError(err)
			}
			if strings.TrimSpace(linearUserID) != "" {
				assigneeID = &linearUserID
			}
		}
	}

	dueDisplay := "—"
	var dueDateForLinear *string
	if req.DueAt != nil {
		formattedDueDate := req.DueAt.In(loc).Format("2006-01-02")
		dueDisplay = formattedDueDate
		dueDateForLinear = &formattedDueDate
	}

	var stateID *string
	desiredStateType := linearStateTypeForRequestStatus(req.Status)
	if desiredStateType != "" {
		var states []linear.WorkflowState
		if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
			fetchedStates, err := d.linearClient.ListTeamStates(ctx, token, *policy.LinearTeamID)
			if err != nil {
				return err
			}
			states = fetchedStates
			return nil
		}); err != nil {
			return classifyLinearError(err)
		}
		if selectedStateID := pickLinearStateID(states, desiredStateType); selectedStateID != "" {
			stateID = &selectedStateID
		}
	}

	descriptionLines := []string{
		req.BodyText,
		"",
		"*TriageGuard metadata*",
		fmt.Sprintf("- Status: %s", db.DerivedStatus(req)),
		fmt.Sprintf("- Priority: %s", req.Priority),
		fmt.Sprintf("- Type: %s", req.RequestType),
		fmt.Sprintf("- Assignee (Slack): %s", ownerDisplay),
		fmt.Sprintf("- Due date: %s", dueDisplay),
	}
	if threadURL != "" {
		descriptionLines = append(descriptionLines, "- Slack thread: "+threadURL)
	}

	var issue linear.CreatedIssue
	if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
		createdIssue, err := d.linearClient.CreateIssue(ctx, token, linear.CreateIssueInput{
			TeamID:      *policy.LinearTeamID,
			Title:       title,
			Description: strings.Join(descriptionLines, "\n"),
			AssigneeID:  assigneeID,
			DueDate:     dueDateForLinear,
			StateID:     stateID,
		})
		if err != nil {
			return err
		}
		issue = createdIssue
		return nil
	}); err != nil {
		return classifyLinearError(err)
	}

	if assigneeID != nil || dueDateForLinear != nil || stateID != nil {
		if err := d.withLinearAccessToken(ctx, req.WorkspaceID, func(token string) error {
			return d.linearClient.UpdateIssue(ctx, token, linear.UpdateIssueInput{
				IssueID:     issue.ID,
				AssigneeID:  assigneeID,
				DueDate:     dueDateForLinear,
				StateID:     stateID,
				SetAssignee: assigneeID != nil,
				SetDueDate:  dueDateForLinear != nil,
				SetState:    stateID != nil,
			})
		}); err != nil {
			log.Printf("outbox: tracker link fallback update failed request_id=%s issue_id=%s: %v", req.ID, issue.ID, err)
		}
	}

	if err := d.store.UpsertLinearIssue(ctx, db.LinearIssue{RequestID: req.ID, IssueID: issue.ID, IssueURL: issue.URL}); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, _ = d.store.InsertRequestEvent(ctx, db.RequestEvent{
		RequestID:  req.ID,
		EventType:  "external_issue_linked",
		ActorType:  "scheduler",
		OccurredAt: now,
		Payload:    mustJSON(map[string]any{"provider": "linear", "external_id": issue.ID, "external_url": issue.URL}),
	})
	if _, err := d.store.InsertOutboxItem(ctx, db.NotificationOutboxItem{
		RequestID:       &req.ID,
		Kind:            "triage_card_refresh",
		DestinationType: "request",
		Payload:         mustJSON(map[string]any{"request_id": req.ID.String()}),
		DedupeKey:       stringPtr(fmt.Sprintf("triage_card_refresh:%s", req.ID)),
		Status:          "pending",
		AvailableAt:     now,
	}); err != nil {
		log.Printf("outbox: enqueue refresh after tracker link failed request=%s: %v", req.ID, err)
	}
	if _, err := d.store.InsertOutboxItem(ctx, db.NotificationOutboxItem{
		RequestID:       &req.ID,
		Kind:            "external_sync_request",
		DestinationType: "request",
		Payload:         mustJSON(map[string]any{"request_id": req.ID.String(), "source": "tracker_link"}),
		DedupeKey:       stringPtr(fmt.Sprintf("external_sync:%s", req.ID)),
		Status:          "pending",
		AvailableAt:     now,
	}); err != nil {
		log.Printf("outbox: enqueue external sync after tracker link failed request=%s: %v", req.ID, err)
	}
	return nil
}

func (d *NotificationDispatcher) refreshTriageCard(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID) error {
	req, err := d.store.GetRequestByID(ctx, requestID)
	if err != nil {
		return err
	}
	queueLabel := "No queue"
	if req.QueueID != nil {
		if queue, queueErr := d.store.GetQueueByID(ctx, *req.QueueID); queueErr == nil {
			queueLabel = queue.Name
		}
	}
	externalIssues, _ := d.store.ListExternalIssuesByRequest(ctx, req.ID)
	trackerText := ""
	if len(externalIssues) > 0 {
		issue := externalIssues[0]
		label := strings.ToUpper(issue.Provider)
		if issue.ExternalKey != nil && strings.TrimSpace(*issue.ExternalKey) != "" {
			label = *issue.ExternalKey
		}
		if issue.ExternalURL != nil && strings.TrimSpace(*issue.ExternalURL) != "" {
			trackerText = fmt.Sprintf("*Tracker:* <%s|%s>", *issue.ExternalURL, label)
		} else {
			trackerText = fmt.Sprintf("*Tracker:* %s", label)
		}
	}
	clocks, _ := d.store.ListRequestSLAClocks(ctx, req.ID)
	slaHint := slackapi.RenderSLAHint(req, clocks, time.Now().UTC())
	blocks := slackapi.RenderTriageBlocks(req, queueLabel, trackerText, slaHint)
	if req.TriageMessageTS == nil || *req.TriageMessageTS == "" {
		posted, err := d.slackClient.PostMessage(ctx, install.BotToken, req.ChannelID, "TriageGuard Request", req.ThreadTS, blocks)
		if err != nil {
			return err
		}
		return d.store.SaveTriageMessageTS(ctx, req.ID, posted.TS)
	}
	return d.slackClient.UpdateMessage(ctx, install.BotToken, req.ChannelID, *req.TriageMessageTS, "TriageGuard Request", blocks)
}

func (d *NotificationDispatcher) outboxRequestID(item db.NotificationOutboxItem) (uuid.UUID, error) {
	if item.RequestID != nil {
		return *item.RequestID, nil
	}
	var payload struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(item.Payload, &payload); err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(strings.TrimSpace(payload.RequestID))
}

func (d *NotificationDispatcher) withLinearAccessToken(ctx context.Context, workspaceID uuid.UUID, operation func(token string) error) error {
	install, err := d.store.GetLinearInstallation(ctx, workspaceID)
	if err != nil {
		return err
	}

	now := time.Now()
	if shouldRefreshLinearToken(install, now) {
		refreshed, refreshErr := d.refreshLinearInstallation(ctx, workspaceID, install)
		if refreshErr != nil {
			if linear.IsAuthError(refreshErr) {
				return terminalOutboxError{err: fmt.Errorf("linear auth required: reconnect Linear in Admin Console")}
			}
			return refreshErr
		}
		install = refreshed
	}

	runErr := operation(install.AccessToken)
	if runErr == nil {
		return nil
	}
	if !linear.IsAuthError(runErr) {
		return runErr
	}

	refreshed, refreshErr := d.refreshLinearInstallation(ctx, workspaceID, install)
	if refreshErr != nil {
		if linear.IsAuthError(refreshErr) {
			return terminalOutboxError{err: fmt.Errorf("linear auth required: reconnect Linear in Admin Console")}
		}
		return refreshErr
	}
	retryErr := operation(refreshed.AccessToken)
	if retryErr != nil {
		if linear.IsAuthError(retryErr) {
			return terminalOutboxError{err: fmt.Errorf("linear auth required: reconnect Linear in Admin Console")}
		}
		return retryErr
	}
	return nil
}

func (d *NotificationDispatcher) refreshLinearInstallation(ctx context.Context, workspaceID uuid.UUID, install db.LinearInstallation) (db.LinearInstallation, error) {
	if install.RefreshToken == nil || strings.TrimSpace(*install.RefreshToken) == "" {
		return db.LinearInstallation{}, terminalOutboxError{err: fmt.Errorf("%w: refresh token is missing", linear.ErrNotAuthenticated)}
	}
	if strings.TrimSpace(d.cfg.LinearClientID) == "" || strings.TrimSpace(d.cfg.LinearClientSecret) == "" {
		return db.LinearInstallation{}, terminalOutboxError{err: fmt.Errorf("linear oauth client is not configured")}
	}
	refreshed, err := d.linearClient.RefreshOAuthToken(ctx, d.cfg.LinearClientID, d.cfg.LinearClientSecret, *install.RefreshToken)
	if err != nil {
		return db.LinearInstallation{}, err
	}
	if err := d.store.UpsertLinearInstallation(ctx, workspaceID, refreshed.AccessToken, optionalStringPtr(refreshed.RefreshToken), linearTokenExpiresAt(time.Now(), refreshed.ExpiresIn)); err != nil {
		return db.LinearInstallation{}, err
	}
	return d.store.GetLinearInstallation(ctx, workspaceID)
}

func (d *NotificationDispatcher) recordDeadLetter(ctx context.Context, item db.NotificationOutboxItem, err error) {
	if err == nil {
		return
	}
	rawPayload := json.RawMessage(`{}`)
	if len(item.Payload) > 0 {
		rawPayload = item.Payload
	}
	_ = d.store.InsertDeadLetter(ctx, db.DeadLetter{
		Source:       "notification_outbox",
		Operation:    item.Kind,
		WorkspaceID:  nil,
		RequestID:    item.RequestID,
		ExternalID:   nil,
		ErrorMessage: strings.TrimSpace(err.Error()),
		Payload:      rawPayload,
	})
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(attempt*attempt) * time.Minute
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}

func classifyLinearError(err error) error {
	if err == nil {
		return nil
	}
	if isTerminalOutboxError(err) {
		return err
	}
	if linear.IsAuthError(err) {
		return terminalOutboxError{err: err}
	}
	return err
}

func isTerminalOutboxError(err error) bool {
	var target terminalOutboxError
	return errors.As(err, &target)
}

func errorsIsNoRows(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "no rows")
}

func buildThreadURL(domain, channelID, threadTS string) string {
	ts := strings.ReplaceAll(threadTS, ".", "")
	if domain == "" || channelID == "" || ts == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", domain, channelID, ts)
}

func linearTokenExpiresAt(now time.Time, expiresIn int) *time.Time {
	if expiresIn <= 0 {
		return nil
	}
	expiresAt := now.UTC().Add(time.Duration(expiresIn) * time.Second)
	return &expiresAt
}

func shouldRefreshLinearToken(install db.LinearInstallation, now time.Time) bool {
	if install.TokenExpiresAt == nil {
		return false
	}
	return install.TokenExpiresAt.Before(now.UTC().Add(2 * time.Minute))
}

func linearStateTypeForRequestStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ASSIGNED", "IN_PROGRESS":
		return "started"
	case "RESOLVED":
		return "completed"
	case "IGNORED":
		return "canceled"
	case "NEW", "ACKED":
		return "unstarted"
	default:
		return "unstarted"
	}
}

func pickLinearStateID(states []linear.WorkflowState, desiredType string) string {
	if len(states) == 0 {
		return ""
	}
	normalizedDesired := strings.ToLower(strings.TrimSpace(desiredType))
	candidates := []string{normalizedDesired}
	switch normalizedDesired {
	case "started":
		candidates = append(candidates, "unstarted", "backlog")
	case "unstarted":
		candidates = append(candidates, "backlog")
	case "completed":
		candidates = append(candidates, "started", "unstarted")
	case "canceled":
		candidates = append(candidates, "completed", "started", "unstarted")
	}
	for _, candidate := range candidates {
		for _, state := range states {
			if strings.ToLower(strings.TrimSpace(state.Type)) == candidate {
				return state.ID
			}
		}
	}
	for _, state := range states {
		if strings.ToLower(strings.TrimSpace(state.Type)) != "backlog" {
			return state.ID
		}
	}
	return states[0].ID
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
	value := dueAt.In(loc).Format("2006-01-02")
	return &value
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
