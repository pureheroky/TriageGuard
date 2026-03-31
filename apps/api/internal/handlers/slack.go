package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
	"triageguard/apps/api/internal/slack"
)

type slackEventEnvelope struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	TeamID    string          `json:"team_id"`
	Event     json.RawMessage `json:"event"`
}

type slackMessageEvent struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	Subtype  string `json:"subtype"`
}

type slackEventType struct {
	Type string `json:"type"`
}

type slackTokensRevokedEvent struct {
	Type   string `json:"type"`
	Tokens struct {
		OAuth []string `json:"oauth"`
		Bot   []string `json:"bot"`
	} `json:"tokens"`
}

type slackActionPayload struct {
	Type      string `json:"type"`
	TriggerID string `json:"trigger_id"`
	Team      struct {
		ID string `json:"id"`
	} `json:"team"`
	User struct {
		ID string `json:"id"`
	} `json:"user"`
	Channel struct {
		ID string `json:"id"`
	} `json:"channel"`
	Container struct {
		ChannelID string `json:"channel_id"`
		MessageTS string `json:"message_ts"`
	} `json:"container"`
	Actions []struct {
		ActionID       string `json:"action_id"`
		Value          string `json:"value"`
		ActionTS       string `json:"action_ts"`
		SelectedOption struct {
			Value string `json:"value"`
		} `json:"selected_option"`
	} `json:"actions"`
	View struct {
		ID              string `json:"id"`
		CallbackID      string `json:"callback_id"`
		PrivateMetadata string `json:"private_metadata"`
		State           struct {
			Values map[string]map[string]struct {
				SelectedUser   string `json:"selected_user"`
				SelectedDate   string `json:"selected_date"`
				Value          string `json:"value"`
				SelectedOption struct {
					Value string `json:"value"`
				} `json:"selected_option"`
				SelectedOptions []struct {
					Value string `json:"value"`
				} `json:"selected_options"`
			} `json:"values"`
		} `json:"state"`
	} `json:"view"`
}

func (a *App) handleSlackInstall(w http.ResponseWriter, r *http.Request) {
	redirectURI := strings.TrimSuffix(a.cfg.APIBaseURL, "/") + "/slack/oauth/callback"

	parsedUserID, err := userIDFromContext(r.Context())
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		http.Error(w, "workspace not found", http.StatusNotFound)
		return
	}
	allowed, _, _, err := a.workspaceHasPaidAccess(r.Context(), workspace.ID)
	if err != nil {
		http.Error(w, "failed to check subscription", http.StatusInternalServerError)
		return
	}
	if !allowed {
		jsonResponse(w, http.StatusPaymentRequired, map[string]any{
			"error": "active paid subscription required. Open Billing and activate Team or Enterprise plan.",
		})
		return
	}

	userID := &parsedUserID
	workspaceID := &workspace.ID
	state, nonce, err := a.buildOAuthState("slack", userID, workspaceID)
	if err != nil {
		http.Error(w, "failed to start slack oauth", http.StatusInternalServerError)
		return
	}
	a.setOAuthStateCookie(w, "slack", nonce)

	u, _ := url.Parse("https://slack.com/oauth/v2/authorize")
	q := u.Query()
	q.Set("client_id", a.cfg.SlackClientID)
	q.Set("scope", "chat:write,channels:read,channels:history,channels:join,groups:read,groups:history,users:read,users:read.email")
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (a *App) handleSlackOAuthCallback(w http.ResponseWriter, r *http.Request) {
	defer a.clearOAuthStateCookie(w, "slack")
	statePayload, err := a.validateOAuthState(r, "slack")
	if err != nil {
		a.logger.Printf("slack oauth callback: invalid state: %v", err)
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	a.logger.Printf("slack oauth callback: code_received=true")

	redirectURI := strings.TrimSuffix(a.cfg.APIBaseURL, "/") + "/slack/oauth/callback"
	resp, err := a.slackClient.ExchangeOAuthCode(r.Context(), a.cfg.SlackClientID, a.cfg.SlackClientSecret, code, redirectURI)
	if err != nil {
		a.logger.Printf("slack oauth exchange failed: %v", err)
		http.Error(w, "slack oauth exchange failed", http.StatusBadGateway)
		return
	}
	accessToken, _ := resp["access_token"].(string)
	botUserID, _ := resp["bot_user_id"].(string)
	teamID := nestedString(resp, "team", "id")
	teamName := nestedString(resp, "team", "name")
	teamDomain := nestedString(resp, "team", "domain")
	if teamID == "" || accessToken == "" || botUserID == "" {
		a.logger.Printf("slack oauth callback: incomplete oauth response team_id=%q bot_user_id_present=%t access_token_present=%t", teamID, botUserID != "", accessToken != "")
		http.Error(w, "incomplete oauth response", http.StatusBadGateway)
		return
	}
	workspace, err := func() (db.Workspace, error) {
		if strings.TrimSpace(statePayload.WorkspaceID) == "" {
			return a.store.UpsertSlackInstallation(r.Context(), teamID, teamName, teamDomain, botUserID, accessToken)
		}
		workspaceID, err := uuid.Parse(strings.TrimSpace(statePayload.WorkspaceID))
		if err != nil {
			return db.Workspace{}, err
		}
		return a.store.UpsertSlackInstallationForWorkspace(r.Context(), workspaceID, teamID, teamName, teamDomain, botUserID, accessToken)
	}()
	if err != nil {
		a.logger.Printf("slack oauth db upsert failed team_id=%s team_name=%s: %v", teamID, teamName, err)
		http.Error(w, "failed to save slack installation (check api db connectivity)", http.StatusInternalServerError)
		return
	}
	if statePayload.UserID != "" {
		userID, err := uuid.Parse(statePayload.UserID)
		if err != nil {
			a.logger.Printf("slack oauth callback: invalid state user id %q: %v", statePayload.UserID, err)
			http.Error(w, "invalid oauth state user", http.StatusBadRequest)
			return
		}
		if err := a.store.EnsureWorkspaceMember(r.Context(), workspace.ID, userID, "owner"); err != nil {
			a.logger.Printf("slack oauth callback: failed to bind workspace membership workspace=%s user=%s: %v", workspace.ID, userID, err)
			http.Error(w, "failed to bind workspace owner", http.StatusInternalServerError)
			return
		}
	}
	a.logger.Printf("slack oauth connected team_id=%s team_name=%s", teamID, teamName)
	http.Redirect(w, r, strings.TrimSuffix(a.cfg.WebBaseURL, "/")+"/onboarding?connected=slack", http.StatusFound)
}

func (a *App) handleSlackEvents(w http.ResponseWriter, r *http.Request) {
	body, err := slack.VerifyRequest(r, a.cfg.SlackSigningSecret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var env slackEventEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	if env.Type == "url_verification" {
		jsonResponse(w, http.StatusOK, map[string]any{"challenge": env.Challenge})
		return
	}
	if env.Type != "event_callback" {
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true})

	go func(teamID string, rawEvent json.RawMessage) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := a.processSlackEvent(ctx, teamID, rawEvent); err != nil {
			a.logger.Printf("slack event process error: %v", err)
		}
	}(env.TeamID, env.Event)
}

func (a *App) processSlackEvent(ctx context.Context, teamID string, rawEvent json.RawMessage) error {
	var eventType slackEventType
	if err := json.Unmarshal(rawEvent, &eventType); err != nil {
		return nil
	}

	switch strings.TrimSpace(eventType.Type) {
	case "message":
		var event slackMessageEvent
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			return nil
		}
		return a.processSlackMessageEvent(ctx, teamID, event)
	case "app_uninstalled":
		return a.cleanupSlackWorkspace(ctx, teamID, "app_uninstalled")
	case "tokens_revoked":
		var event slackTokensRevokedEvent
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			return nil
		}
		if len(event.Tokens.Bot) == 0 && len(event.Tokens.OAuth) > 0 {
			a.logger.Printf("slack tokens_revoked ignored team=%s oauth_count=%d bot_count=0", teamID, len(event.Tokens.OAuth))
			return nil
		}
		return a.cleanupSlackWorkspace(ctx, teamID, "tokens_revoked")
	default:
		return nil
	}
}

func (a *App) cleanupSlackWorkspace(ctx context.Context, teamID, reason string) error {
	workspace, err := a.store.GetWorkspaceByTeamID(ctx, teamID)
	if err != nil {
		if errorsIsNoRows(err) {
			return nil
		}
		return err
	}

	slackDisconnected, err := a.store.DisconnectSlack(ctx, workspace.ID)
	if err != nil {
		return err
	}
	linearDisconnected, err := a.store.DisconnectLinear(ctx, workspace.ID)
	if err != nil && !errorsIsNoRows(err) {
		return err
	}
	a.logger.Printf(
		"slack lifecycle cleanup reason=%s team_id=%s workspace_id=%s slack_disconnected=%t linear_disconnected=%t",
		reason,
		teamID,
		workspace.ID,
		slackDisconnected,
		linearDisconnected,
	)
	return nil
}

func (a *App) processSlackMessageEvent(ctx context.Context, teamID string, event slackMessageEvent) error {
	if event.Type != "message" || event.Subtype != "" || event.User == "" {
		return nil
	}
	isThreadReply := event.ThreadTS != "" && event.ThreadTS != event.TS
	threadTS := event.TS
	if isThreadReply {
		threadTS = event.ThreadTS
	}
	install, err := a.store.GetSlackInstallationByTeam(ctx, teamID)
	if err != nil {
		return err
	}
	allowed, _, _, err := a.workspaceHasPaidAccess(ctx, install.WorkspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		return nil
	}
	enabled, err := a.store.IsChannelEnabled(ctx, install.WorkspaceID, event.Channel)
	if err != nil || !enabled {
		return err
	}
	if isThreadReply {
		sourceKey := teamID + ":" + event.Channel + ":" + threadTS
		return a.store.TouchRequestBySourceKey(ctx, sourceKey)
	}
	title := event.Text
	if len([]rune(title)) > 80 {
		title = strings.TrimSpace(string([]rune(title)[:80]))
	}
	titlePtr := optionalString(title)
	req, err := a.store.UpsertRequest(ctx, db.Request{
		WorkspaceID:   install.WorkspaceID,
		SourceKey:     teamID + ":" + event.Channel + ":" + threadTS,
		ChannelID:     event.Channel,
		ThreadTS:      threadTS,
		MessageTS:     event.TS,
		AuthorSlackID: event.User,
		BodyText:      strings.TrimSpace(event.Text),
		Title:         titlePtr,
	})
	if err != nil {
		return err
	}
	return a.refreshTriageCard(ctx, install, req.ID)
}

func (a *App) handleSlackActions(w http.ResponseWriter, r *http.Request) {
	body, err := slack.VerifyRequest(r, a.cfg.SlackSigningSecret)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	values, err := url.ParseQuery(string(body))
	if err != nil {
		http.Error(w, "bad form payload", http.StatusBadRequest)
		return
	}
	payloadJSON := values.Get("payload")
	if payloadJSON == "" {
		http.Error(w, "missing payload", http.StatusBadRequest)
		return
	}
	var payload slackActionPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		http.Error(w, "bad payload", http.StatusBadRequest)
		return
	}
	if dedupeKey := slackActionDedupKey(payload); dedupeKey != "" {
		recorded, err := a.store.TryRecordSlackActionDedup(r.Context(), dedupeKey)
		if err != nil {
			a.logger.Printf("slack action dedupe insert failed: %v", err)
		} else if !recorded {
			jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}

	switch payload.Type {
	case "block_actions":
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
		go func(payload slackActionPayload) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			if len(payload.Actions) == 0 {
				return
			}
			if err := a.handleBlockAction(ctx, payload); err != nil {
				a.logger.Printf("block action error: %v", err)
				a.notifySlackActionError(ctx, payload, err)
			}
		}(payload)
		return
	case "view_submission":
		jsonResponse(w, http.StatusOK, map[string]any{})
		go func(payload slackActionPayload) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := a.handleViewSubmission(ctx, payload); err != nil {
				a.logger.Printf("view submission error: %v", err)
				a.notifySlackActionError(ctx, payload, err)
			}
		}(payload)
		return
	default:
		jsonResponse(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
}

func (a *App) handleBlockAction(ctx context.Context, payload slackActionPayload) error {
	action := payload.Actions[0]
	install, err := a.store.GetSlackInstallationByTeam(ctx, payload.Team.ID)
	if err != nil {
		return err
	}
	allowed, _, _, err := a.workspaceHasPaidAccess(ctx, install.WorkspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		channelID := strings.TrimSpace(payload.Channel.ID)
		if channelID == "" {
			channelID = strings.TrimSpace(payload.Container.ChannelID)
		}
		if channelID == "" || strings.TrimSpace(payload.User.ID) == "" {
			return nil
		}
		return a.slackClient.PostEphemeral(
			ctx,
			install.BotToken,
			channelID,
			payload.User.ID,
			"Team or Enterprise subscription required. Open Billing in TriageGuard and activate a plan.",
		)
	}

	channelID := strings.TrimSpace(payload.Channel.ID)
	if channelID == "" {
		channelID = strings.TrimSpace(payload.Container.ChannelID)
	}
	workspaceID := install.WorkspaceID

	switch action.ActionID {
	case "tg_triage":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		request, err := a.store.GetRequestByID(ctx, requestID)
		if err != nil {
			return err
		}
		queues, err := a.store.ListQueues(ctx, request.WorkspaceID)
		if err != nil {
			return err
		}
		return a.slackClient.OpenView(ctx, install.BotToken, payload.TriggerID, slack.RenderTriageModal(request, queues))
	case "tg_ack":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		ack := true
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:   requestID,
			ActorType:   "slack_user",
			ActorID:     optionalString(payload.User.ID),
			Acknowledge: &ack,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "ACK"})
		return a.refreshTriageCard(ctx, install, requestID)
	case "tg_assign_me":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		owner := strings.TrimSpace(payload.User.ID)
		if owner == "" {
			return nil
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:   requestID,
			ActorType:   "slack_user",
			ActorID:     optionalString(payload.User.ID),
			OwnerUserID: &owner,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "ASSIGN_ME"})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, channelID, payload.User.ID)
		return nil
	case "tg_waiting_requester":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		waitingOn := db.WaitingOnRequester
		comment := "Waiting on requester"
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID: requestID,
			ActorType: "slack_user",
			ActorID:   optionalString(payload.User.ID),
			WaitingOn: &waitingOn,
			Comment:   &comment,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "WAITING_REQUESTER"})
		return a.refreshTriageCard(ctx, install, requestID)
	case "tg_snooze_tomorrow":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		request, err := a.store.GetRequestByID(ctx, requestID)
		if err != nil {
			return err
		}
		snoozedUntil, err := a.resolveSnoozeSelection(ctx, request, "tomorrow", "")
		if err != nil {
			return err
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:    requestID,
			ActorType:    "slack_user",
			ActorID:      optionalString(payload.User.ID),
			SnoozedUntil: &snoozedUntil,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "SNOOZE_TOMORROW"})
		return a.refreshTriageCard(ctx, install, requestID)
	case "tg_resolve":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		reason := db.ClosedReasonResolved
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:   requestID,
			ActorType:   "slack_user",
			ActorID:     optionalString(payload.User.ID),
			CloseReason: &reason,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "RESOLVE"})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, channelID, payload.User.ID)
		return nil
	case "tg_reopen":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID: requestID,
			ActorType: "slack_user",
			ActorID:   optionalString(payload.User.ID),
			Reopen:    true,
		}); err != nil {
			return err
		}
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "REOPEN"})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, channelID, payload.User.ID)
		return nil
	case "tg_more":
		command, requestID, err := parseOverflowAction(action.SelectedOption.Value)
		if err != nil {
			return err
		}
		request, err := a.store.GetRequestByID(ctx, requestID)
		if err != nil {
			return err
		}
		switch command {
		case "waiting":
			return a.slackClient.OpenView(ctx, install.BotToken, payload.TriggerID, slack.RenderWaitingModal(request))
		case "snooze":
			return a.slackClient.OpenView(ctx, install.BotToken, payload.TriggerID, slack.RenderSnoozeModal(request))
		case "reassign":
			queues, err := a.store.ListQueues(ctx, request.WorkspaceID)
			if err != nil {
				return err
			}
			return a.slackClient.OpenView(ctx, install.BotToken, payload.TriggerID, slack.RenderTriageModal(request, queues))
		case "tracker":
			return a.handleTrackerLinkAction(ctx, install, requestID, channelID, payload.User.ID, &workspaceID)
		case "close":
			return a.slackClient.OpenView(ctx, install.BotToken, payload.TriggerID, slack.RenderCloseModal(request))
		case "reopen":
			if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
				RequestID: requestID,
				ActorType: "slack_user",
				ActorID:   optionalString(payload.User.ID),
				Reopen:    true,
			}); err != nil {
				return err
			}
			_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "REOPEN"})
			if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
				return err
			}
			a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, channelID, payload.User.ID)
			return nil
		default:
			return nil
		}
	case "tg_convert":
		requestID, err := parseUUID(action.Value)
		if err != nil {
			return err
		}
		return a.handleTrackerLinkAction(ctx, install, requestID, channelID, payload.User.ID, &workspaceID)
	default:
		return nil
	}
}

func (a *App) handleViewSubmission(ctx context.Context, payload slackActionPayload) error {
	requestID, err := uuid.Parse(payload.View.PrivateMetadata)
	if err != nil {
		return err
	}
	req, err := a.store.GetRequestByID(ctx, requestID)
	if err != nil {
		return err
	}
	install, err := a.store.GetSlackInstallationByWorkspace(ctx, req.WorkspaceID)
	if err != nil {
		return err
	}
	allowed, _, _, err := a.workspaceHasPaidAccess(ctx, install.WorkspaceID)
	if err != nil {
		return err
	}
	if !allowed {
		if strings.TrimSpace(payload.User.ID) == "" {
			return nil
		}
		return a.slackClient.PostEphemeral(
			ctx,
			install.BotToken,
			req.ChannelID,
			payload.User.ID,
			"Team or Enterprise subscription required. Open Billing in TriageGuard and activate a plan.",
		)
	}
	workspaceID := install.WorkspaceID

	switch payload.View.CallbackID {
	case "tg_triage_submit":
		ack := checkboxHasValue(payload, "ack_block", "ack_checkbox", "ack")
		owner := strings.TrimSpace(selectedUserValue(payload, "owner_block", "owner_select"))
		priority := strings.TrimSpace(selectedOptionValue(payload, "priority_block", "priority_select"))
		requestType := strings.TrimSpace(selectedOptionValue(payload, "type_block", "type_select"))
		queueIDValue := strings.TrimSpace(selectedOptionValue(payload, "queue_block", "queue_select"))
		waitingOn := strings.TrimSpace(selectedOptionValue(payload, "waiting_block", "waiting_select"))
		dueSelected := strings.TrimSpace(selectedDateValue(payload, "due_block", "due_picker"))
		snoozeSelected := strings.TrimSpace(selectedDateValue(payload, "snooze_block", "snooze_picker"))
		linkTracker := checkboxHasValue(payload, "tracker_block", "tracker_checkbox", "link_tracker")

		var queueID *uuid.UUID
		if queueIDValue != "" {
			parsedQueueID, err := uuid.Parse(queueIDValue)
			if err != nil {
				return err
			}
			queueID = &parsedQueueID
		}
		var dueAt *time.Time
		if dueSelected != "" {
			resolvedDueAt, err := a.parseSlackSelectedDate(ctx, req, queueID, dueSelected)
			if err != nil {
				return err
			}
			dueAt = resolvedDueAt
		}
		var snoozedUntil *time.Time
		if snoozeSelected != "" {
			resolvedSnooze, err := a.parseSlackSelectedDate(ctx, req, queueID, snoozeSelected)
			if err != nil {
				return err
			}
			snoozedUntil = resolvedSnooze
		}
		actorID := optionalString(payload.User.ID)
		mutation := db.RequestMutation{
			RequestID:    requestID,
			ActorType:    "slack_user",
			ActorID:      actorID,
			DueAt:        &dueAt,
			SnoozedUntil: &snoozedUntil,
		}
		if ack {
			mutation.Acknowledge = &ack
		}
		if owner != "" {
			mutation.OwnerUserID = &owner
		}
		if priority != "" {
			mutation.Priority = &priority
		}
		if requestType != "" {
			mutation.RequestType = &requestType
		}
		if queueID != nil {
			mutation.QueueID = queueID
		}
		if waitingOn != "" {
			mutation.WaitingOn = &waitingOn
		}
		if linkTracker {
			mutation.LinkTracker = &linkTracker
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, mutation); err != nil {
			return err
		}
		payloadJSON, _ := json.Marshal(map[string]any{
			"ack":           ack,
			"owner":         owner,
			"priority":      priority,
			"request_type":  requestType,
			"queue_id":      queueIDValue,
			"waiting_on":    waitingOn,
			"due_date":      dueSelected,
			"snoozed_until": snoozeSelected,
			"link_tracker":  linkTracker,
		})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "TRIAGE", Payload: payloadJSON})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		if linkTracker {
			return a.handleTrackerLinkAction(ctx, install, requestID, req.ChannelID, payload.User.ID, &workspaceID)
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, req.ChannelID, payload.User.ID)
		return nil
	case "tg_waiting_submit":
		waitingOn := selectedOptionValue(payload, "waiting_block", "waiting_select")
		comment := textInputValue(payload, "comment_block", "comment_input")
		reviewSelected := selectedDateValue(payload, "review_block", "review_picker")
		var snoozedUntil *time.Time
		if reviewSelected != "" {
			resolved, err := a.parseSlackSelectedDate(ctx, req, req.QueueID, reviewSelected)
			if err != nil {
				return err
			}
			snoozedUntil = resolved
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:    requestID,
			ActorType:    "slack_user",
			ActorID:      optionalString(payload.User.ID),
			WaitingOn:    &waitingOn,
			SnoozedUntil: &snoozedUntil,
			Comment:      &comment,
		}); err != nil {
			return err
		}
		payloadJSON, _ := json.Marshal(map[string]any{"waiting_on": waitingOn, "comment": comment, "review_date": reviewSelected})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "WAITING", Payload: payloadJSON})
		return a.refreshTriageCard(ctx, install, requestID)
	case "tg_snooze_submit":
		preset := selectedOptionValue(payload, "preset_block", "preset_select")
		customSelected := selectedDateValue(payload, "custom_block", "custom_picker")
		snoozedUntil, err := a.resolveSnoozeSelection(ctx, req, preset, customSelected)
		if err != nil {
			return err
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:    requestID,
			ActorType:    "slack_user",
			ActorID:      optionalString(payload.User.ID),
			SnoozedUntil: &snoozedUntil,
		}); err != nil {
			return err
		}
		payloadJSON, _ := json.Marshal(map[string]any{"preset": preset, "custom_date": customSelected})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "SNOOZE", Payload: payloadJSON})
		return a.refreshTriageCard(ctx, install, requestID)
	case "tg_close_submit":
		reason := selectedOptionValue(payload, "close_block", "close_select")
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:   requestID,
			ActorType:   "slack_user",
			ActorID:     optionalString(payload.User.ID),
			CloseReason: &reason,
		}); err != nil {
			return err
		}
		payloadJSON, _ := json.Marshal(map[string]any{"closed_reason": reason})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "CLOSE", Payload: payloadJSON})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, req.ChannelID, payload.User.ID)
		return nil
	case "tg_assign_submit":
		owner := payload.View.State.Values["owner_block"]["owner_select"].SelectedUser
		if owner == "" {
			return nil
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID:   requestID,
			ActorType:   "slack_user",
			ActorID:     optionalString(payload.User.ID),
			OwnerUserID: &owner,
		}); err != nil {
			return err
		}
		p, _ := json.Marshal(map[string]any{"owner": owner})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "ASSIGN", Payload: p})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, req.ChannelID, payload.User.ID)
		return nil
	case "tg_due_submit":
		selected := payload.View.State.Values["due_block"]["due_picker"].SelectedDate
		var dueAt *time.Time
		if selected != "" {
			policy, err := a.store.GetPolicy(ctx, req.WorkspaceID)
			if err != nil {
				return err
			}
			loc, err := time.LoadLocation(policy.Timezone)
			if err != nil {
				loc = time.UTC
			}
			t, err := time.ParseInLocation("2006-01-02", selected, loc)
			if err != nil {
				return err
			}
			eod := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc).UTC()
			dueAt = &eod
		}
		if _, _, err := a.store.ApplyRequestMutation(ctx, db.RequestMutation{
			RequestID: requestID,
			ActorType: "slack_user",
			ActorID:   optionalString(payload.User.ID),
			DueAt:     &dueAt,
		}); err != nil {
			return err
		}
		p, _ := json.Marshal(map[string]any{"due_date": selected})
		_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: payload.User.ID, ActionType: "DUE", Payload: p})
		if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
			return err
		}
		a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, &workspaceID, req.ChannelID, payload.User.ID)
		return nil
	default:
		return nil
	}
}

func selectedUserValue(payload slackActionPayload, blockID, actionID string) string {
	return payload.View.State.Values[blockID][actionID].SelectedUser
}

func selectedDateValue(payload slackActionPayload, blockID, actionID string) string {
	return payload.View.State.Values[blockID][actionID].SelectedDate
}

func selectedOptionValue(payload slackActionPayload, blockID, actionID string) string {
	return payload.View.State.Values[blockID][actionID].SelectedOption.Value
}

func textInputValue(payload slackActionPayload, blockID, actionID string) string {
	return payload.View.State.Values[blockID][actionID].Value
}

func checkboxHasValue(payload slackActionPayload, blockID, actionID, expected string) bool {
	for _, option := range payload.View.State.Values[blockID][actionID].SelectedOptions {
		if strings.TrimSpace(option.Value) == strings.TrimSpace(expected) {
			return true
		}
	}
	return false
}

func parseOverflowAction(raw string) (string, uuid.UUID, error) {
	parts := strings.SplitN(strings.TrimSpace(raw), "|", 2)
	if len(parts) != 2 {
		return "", uuid.Nil, fmt.Errorf("invalid overflow action")
	}
	requestID, err := uuid.Parse(parts[1])
	if err != nil {
		return "", uuid.Nil, err
	}
	return strings.TrimSpace(parts[0]), requestID, nil
}

func (a *App) parseSlackSelectedDate(ctx context.Context, req db.Request, queueID *uuid.UUID, selected string) (*time.Time, error) {
	timezone := "UTC"
	if queueID != nil {
		if policy, err := a.store.GetQueuePolicy(ctx, *queueID); err == nil && strings.TrimSpace(policy.Timezone) != "" {
			timezone = policy.Timezone
		}
	} else if req.QueueID != nil {
		if policy, err := a.store.GetQueuePolicy(ctx, *req.QueueID); err == nil && strings.TrimSpace(policy.Timezone) != "" {
			timezone = policy.Timezone
		}
	}
	if timezone == "UTC" {
		if policy, err := a.store.GetPolicy(ctx, req.WorkspaceID); err == nil && strings.TrimSpace(policy.Timezone) != "" {
			timezone = policy.Timezone
		}
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02", selected, loc)
	if err != nil {
		return nil, err
	}
	eod := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, loc).UTC()
	return &eod, nil
}

func (a *App) resolveSnoozeSelection(ctx context.Context, req db.Request, preset, customSelected string) (*time.Time, error) {
	now := time.Now().UTC()
	switch strings.TrimSpace(preset) {
	case "4_hours":
		value := now.Add(4 * time.Hour)
		return &value, nil
	case "tomorrow":
		return a.nextBusinessStart(ctx, req, now)
	case "next_monday":
		loc := time.UTC
		local := now.In(loc)
		for i := 1; i <= 7; i++ {
			candidate := local.AddDate(0, 0, i)
			if candidate.Weekday() == time.Monday {
				eod := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 23, 59, 59, 0, loc).UTC()
				return &eod, nil
			}
		}
	case "custom":
		if customSelected != "" {
			return a.parseSlackSelectedDate(ctx, req, req.QueueID, customSelected)
		}
	}
	if customSelected != "" {
		return a.parseSlackSelectedDate(ctx, req, req.QueueID, customSelected)
	}
	return nil, nil
}

func (a *App) nextBusinessStart(ctx context.Context, req db.Request, now time.Time) (*time.Time, error) {
	policy := db.QueuePolicy{}
	if req.QueueID != nil {
		loaded, err := a.store.GetQueuePolicy(ctx, *req.QueueID)
		if err == nil {
			policy = loaded
			return nextBusinessStartForPolicy(loaded, now), nil
		}
	}
	return nextBusinessStartForPolicy(policy, now), nil
}

func nextBusinessStartForPolicy(policy db.QueuePolicy, now time.Time) *time.Time {
	loc := time.UTC
	if strings.TrimSpace(policy.Timezone) != "" {
		if zone, err := time.LoadLocation(policy.Timezone); err == nil {
			loc = zone
		}
	}
	local := now.In(loc)
	if policy.BusinessHoursEnabled {
		startHour, startMinute, startSecond := 9, 0, 0
		if policy.BusinessHoursStart != nil {
			if parsed, err := time.ParseInLocation("15:04:05", *policy.BusinessHoursStart, loc); err == nil {
				startHour, startMinute, startSecond = parsed.Hour(), parsed.Minute(), parsed.Second()
			}
		}
		for offset := 1; offset <= 7; offset++ {
			candidate := local.AddDate(0, 0, offset)
			weekdayMask := 1 << int(candidate.Weekday())
			if policy.BusinessDaysMask != 0 && policy.BusinessDaysMask&weekdayMask == 0 {
				continue
			}
			value := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), startHour, startMinute, startSecond, 0, loc).UTC()
			return &value
		}
	}
	candidate := local.AddDate(0, 0, 1)
	value := time.Date(candidate.Year(), candidate.Month(), candidate.Day(), 9, 0, 0, 0, loc).UTC()
	return &value
}

func (a *App) handleTrackerLinkAction(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID, channelID, slackUserID string, workspaceID *uuid.UUID) error {
	a.logger.Printf("tracker link: started request_id=%s team=%s user=%s", requestID, install.TeamID, slackUserID)
	if existing, err := a.store.GetLinearIssueByRequest(ctx, requestID); err == nil && existing.IssueURL != "" {
		a.logger.Printf("tracker link: already linked request_id=%s issue_url=%s", requestID, existing.IssueURL)
		return a.refreshTriageCard(ctx, install, requestID)
	} else if err != nil && !errorsIsNoRows(err) {
		return err
	}
	request, err := a.store.GetRequestByID(ctx, requestID)
	if err != nil {
		return err
	}
	if request.State == db.RequestStateClosed {
		return nil
	}
	if _, err := a.store.GetLinearInstallation(ctx, request.WorkspaceID); err != nil {
		if errorsIsNoRows(err) {
			return a.slackClient.PostEphemeral(ctx, install.BotToken, request.ChannelID, slackUserID, "Connect Linear in Admin Console.")
		}
		return err
	}
	policy, err := a.store.GetPolicy(ctx, request.WorkspaceID)
	if err != nil {
		return err
	}
	if policy.LinearTeamID == nil || *policy.LinearTeamID == "" {
		return a.slackClient.PostEphemeral(ctx, install.BotToken, request.ChannelID, slackUserID, "Set Linear team in Admin Console first.")
	}
	threadURL := buildThreadURL(install.TeamDomain, request.ChannelID, request.ThreadTS)
	title := "Triage: "
	if request.Title != nil && *request.Title != "" {
		title += *request.Title
	} else {
		title += request.BodyText
	}

	loc := time.UTC
	if policy.Timezone != "" {
		if loadedLoc, err := time.LoadLocation(policy.Timezone); err == nil {
			loc = loadedLoc
		}
	}

	ownerDisplay := "Unassigned"
	callLinear := func(run func(token string) error) error {
		return a.withLinearAccessToken(ctx, request.WorkspaceID, run)
	}

	var assigneeID *string
	if request.OwnerUserID != nil && *request.OwnerUserID != "" {
		ownerID := strings.TrimSpace(*request.OwnerUserID)
		ownerDisplay = ownerID
		displayName, displayErr := a.slackClient.GetUserDisplayName(ctx, install.BotToken, ownerID)
		if displayErr != nil {
			a.logger.Printf("tracker link: resolve slack owner display name failed request_id=%s owner=%s: %v", request.ID, ownerID, displayErr)
		} else if strings.TrimSpace(displayName) != "" {
			ownerDisplay = strings.TrimSpace(displayName)
		}
		email, err := a.slackClient.GetUserEmail(ctx, install.BotToken, ownerID)
		if err != nil {
			a.logger.Printf("tracker link: resolve slack owner email failed request_id=%s owner=%s: %v", request.ID, ownerID, err)
		} else if email != "" {
			var linearUserID string
			if err := callLinear(func(token string) error {
				resolvedID, err := a.linearClient.FindUserIDByEmail(ctx, token, email)
				if err != nil {
					return err
				}
				linearUserID = resolvedID
				return nil
			}); err != nil {
				if linear.IsAuthError(err) || strings.Contains(strings.ToLower(err.Error()), "linear auth required") {
					return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
				}
				a.logger.Printf("tracker link: find linear user by email failed request_id=%s email=%s: %v", request.ID, email, err)
			} else if linearUserID != "" {
				assigneeID = &linearUserID
			}
		}
	}

	dueDisplay := "—"
	var dueDateForLinear *string
	if request.DueAt != nil {
		formattedDueDate := request.DueAt.In(loc).Format("2006-01-02")
		dueDisplay = formattedDueDate
		dueDateForLinear = &formattedDueDate
	}

	var stateID *string
	desiredStateType := linearStateTypeForRequestStatus(request.Status)
	if desiredStateType != "" {
		var states []linear.WorkflowState
		if err := callLinear(func(token string) error {
			fetchedStates, err := a.linearClient.ListTeamStates(ctx, token, *policy.LinearTeamID)
			if err != nil {
				return err
			}
			states = fetchedStates
			return nil
		}); err != nil {
			if linear.IsAuthError(err) || strings.Contains(strings.ToLower(err.Error()), "linear auth required") {
				return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
			}
			a.logger.Printf("tracker link: list team states failed request_id=%s: %v", request.ID, err)
		} else if selectedStateID := pickLinearStateID(states, desiredStateType); selectedStateID != "" {
			stateID = &selectedStateID
		}
	}

	descriptionLines := []string{
		request.BodyText,
		"",
		"*TriageGuard metadata*",
		fmt.Sprintf("- Status: %s", db.DerivedStatus(request)),
		fmt.Sprintf("- Priority: %s", request.Priority),
		fmt.Sprintf("- Type: %s", request.RequestType),
		fmt.Sprintf("- Assignee (Slack): %s", ownerDisplay),
		fmt.Sprintf("- Due date: %s", dueDisplay),
	}
	if threadURL != "" {
		descriptionLines = append(descriptionLines, "- Slack thread: "+threadURL)
	}

	var issue linear.CreatedIssue
	if err := callLinear(func(token string) error {
		createdIssue, err := a.linearClient.CreateIssue(ctx, token, linear.CreateIssueInput{
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
		if linear.IsAuthError(err) || strings.Contains(strings.ToLower(err.Error()), "linear auth required") {
			return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
		}
		a.logger.Printf("tracker link: create issue failed request_id=%s: %v", request.ID, err)
		return err
	}
	if assigneeID != nil || dueDateForLinear != nil || stateID != nil {
		if err := callLinear(func(token string) error {
			return a.linearClient.UpdateIssue(ctx, token, linear.UpdateIssueInput{
				IssueID:     issue.ID,
				AssigneeID:  assigneeID,
				DueDate:     dueDateForLinear,
				StateID:     stateID,
				SetAssignee: assigneeID != nil,
				SetDueDate:  dueDateForLinear != nil,
				SetState:    stateID != nil,
			})
		}); err != nil {
			a.logger.Printf("tracker link: issue update fallback failed request_id=%s issue_id=%s: %v", request.ID, issue.ID, err)
		}
	}
	if err := a.store.UpsertLinearIssue(ctx, db.LinearIssue{RequestID: requestID, IssueID: issue.ID, IssueURL: issue.URL}); err != nil {
		return err
	}
	payloadJSON, _ := json.Marshal(map[string]any{
		"provider":        "linear",
		"issue_id":        issue.ID,
		"issue_url":       issue.URL,
		"linear_state_id": stateID,
		"assignee_id":     assigneeID,
		"due_date":        dueDateForLinear,
	})
	_ = a.store.InsertAction(ctx, db.RequestAction{RequestID: requestID, ActorSlackID: slackUserID, ActionType: "TRACKER_LINK", Payload: payloadJSON})
	if err := a.refreshTriageCard(ctx, install, requestID); err != nil {
		return err
	}
	a.trySyncLinkedLinearIssueFromSlackInChannel(ctx, install, requestID, workspaceID, channelID, slackUserID)
	return nil
}

func (a *App) refreshTriageCard(ctx context.Context, install db.SlackInstallation, requestID uuid.UUID) error {
	req, err := a.store.GetRequestByID(ctx, requestID)
	if err != nil {
		return err
	}
	queueLabel := "No queue"
	if req.QueueID != nil {
		if queue, queueErr := a.store.GetQueueByID(ctx, *req.QueueID); queueErr == nil {
			queueLabel = queue.Name
		}
	}
	externalIssues, _ := a.store.ListExternalIssuesByRequest(ctx, req.ID)
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
	clocks, _ := a.store.ListRequestSLAClocks(ctx, req.ID)
	slaHint := slack.RenderSLAHint(req, clocks, time.Now().UTC())
	blocks := slack.RenderTriageBlocks(req, queueLabel, trackerText, slaHint)
	if req.TriageMessageTS == nil || *req.TriageMessageTS == "" {
		posted, err := a.slackClient.PostMessage(ctx, install.BotToken, req.ChannelID, "TriageGuard Request", req.ThreadTS, blocks)
		if err != nil {
			return err
		}
		return a.store.SaveTriageMessageTS(ctx, req.ID, posted.TS)
	}
	return a.slackClient.UpdateMessage(ctx, install.BotToken, req.ChannelID, *req.TriageMessageTS, "TriageGuard Request", blocks)
}

func (a *App) effectiveSLAForChannel(ctx context.Context, workspaceID uuid.UUID, channelID string, policy db.Policy) (int, int, int, error) {
	ackSLA := policy.AckSLAMinutes
	assignSLA := policy.AssignSLAMinutes
	staleHours := policy.StaleHours
	if ackSLA <= 0 {
		ackSLA = 15
	}
	if assignSLA <= 0 {
		assignSLA = 30
	}
	if staleHours <= 0 {
		staleHours = 24
	}

	hasEnterprise, err := a.workspaceHasEnterprise(ctx, workspaceID)
	if err != nil {
		return ackSLA, assignSLA, staleHours, err
	}
	if !hasEnterprise {
		return ackSLA, assignSLA, staleHours, nil
	}

	override, err := a.store.GetChannelSLAOverride(ctx, workspaceID, channelID)
	if err != nil {
		if errorsIsNoRows(err) {
			return ackSLA, assignSLA, staleHours, nil
		}
		return ackSLA, assignSLA, staleHours, err
	}
	if override.AckSLAMinutes > 0 {
		ackSLA = override.AckSLAMinutes
	}
	if override.AssignSLAMinutes > 0 {
		assignSLA = override.AssignSLAMinutes
	}
	if override.StaleHours > 0 {
		staleHours = override.StaleHours
	}
	return ackSLA, assignSLA, staleHours, nil
}

func nestedString(m map[string]any, keys ...string) string {
	cur := any(m)
	for _, key := range keys {
		obj, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur, ok = obj[key]
		if !ok {
			return ""
		}
	}
	s, _ := cur.(string)
	return s
}

func slackActionDedupKey(payload slackActionPayload) string {
	var raw string
	switch payload.Type {
	case "block_actions":
		if len(payload.Actions) == 0 {
			return ""
		}
		action := payload.Actions[0]
		raw = strings.Join([]string{
			"block",
			payload.Team.ID,
			payload.User.ID,
			payload.Container.ChannelID,
			payload.Container.MessageTS,
			action.ActionID,
			action.Value,
			action.SelectedOption.Value,
			action.ActionTS,
		}, "|")
	case "view_submission":
		stateJSON, _ := json.Marshal(payload.View.State.Values)
		raw = strings.Join([]string{
			"view",
			payload.Team.ID,
			payload.User.ID,
			payload.View.ID,
			payload.View.CallbackID,
			payload.View.PrivateMetadata,
			string(stateJSON),
		}, "|")
	default:
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return "slack_action:" + hex.EncodeToString(sum[:])
}

func buildThreadURL(domain, channelID, threadTS string) string {
	ts := strings.ReplaceAll(threadTS, ".", "")
	if domain == "" || channelID == "" || ts == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", domain, channelID, ts)
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

func errorsIsNoRows(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "no rows") || err == pgx.ErrNoRows)
}

func (a *App) notifySlackActionError(ctx context.Context, payload slackActionPayload, err error) {
	install, installErr := a.store.GetSlackInstallationByTeam(ctx, payload.Team.ID)
	if installErr != nil {
		a.logger.Printf("slack action error notify skipped: install lookup failed team=%s: %v", payload.Team.ID, installErr)
		return
	}

	channelID := strings.TrimSpace(payload.Channel.ID)
	if channelID == "" {
		channelID = strings.TrimSpace(payload.Container.ChannelID)
	}
	if channelID == "" {
		return
	}
	userID := strings.TrimSpace(payload.User.ID)
	if userID == "" {
		return
	}

	message := "Action failed. Please try again."
	lower := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(lower, "linear is not connected"):
		message = "Linear is not connected. Connect Linear in Admin Console."
	case strings.Contains(lower, "set linear teamid"):
		message = "Set Linear team in Admin Console first."
	case strings.Contains(lower, "linear auth required"), strings.Contains(lower, "not authenticated"), strings.Contains(lower, "authentication required"):
		message = "Linear session expired or revoked. Reconnect Linear in Admin Console and retry."
	case strings.Contains(lower, "linear issue create failed"):
		message = "Failed to create Linear issue. Check Linear permissions/team and try again."
	case strings.Contains(lower, "linear issue update failed"):
		message = "Linear issue was created, but metadata update failed. Check Linear permissions."
	case strings.Contains(lower, "deadline exceeded"), strings.Contains(lower, "context canceled"):
		message = "Action timed out. Please retry."
	}
	if postErr := a.slackClient.PostEphemeral(ctx, install.BotToken, channelID, userID, message); postErr != nil {
		a.logger.Printf("slack action error notify failed team=%s channel=%s user=%s: %v", payload.Team.ID, channelID, userID, postErr)
	}
}
