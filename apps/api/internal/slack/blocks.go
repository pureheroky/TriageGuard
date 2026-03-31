package slack

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
)

func RenderTriageBlocks(req db.Request, queueLabel string, trackerText string, slaHint string) []map[string]any {
	owner := "Unassigned"
	if req.OwnerUserID != nil && *req.OwnerUserID != "" {
		owner = "<@" + *req.OwnerUserID + ">"
	}
	due := "—"
	if req.DueAt != nil {
		due = req.DueAt.UTC().Format("2006-01-02")
	}
	blocker := blockerLabel(req)

	isClosed := strings.EqualFold(req.State, db.RequestStateClosed)
	resolveLabel := "Resolve"
	resolveActionID := "tg_resolve"
	if isClosed {
		resolveLabel = "Reopen"
		resolveActionID = "tg_reopen"
	}

	actions := []map[string]any{
		button("Triage", "tg_triage", req.ID.String(), "primary", isClosed),
		button("Ack", "tg_ack", req.ID.String(), "", isClosed),
		button(resolveLabel, resolveActionID, req.ID.String(), "", false),
		overflowMenu(req),
	}

	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]any{"type": "plain_text", "text": "TriageGuard Request", "emoji": true},
		},
		{
			"type": "section",
			"fields": []map[string]any{
				{"type": "mrkdwn", "text": "*Owner:* " + owner},
				{"type": "mrkdwn", "text": "*SLA:* " + slaHint},
				{"type": "mrkdwn", "text": "*Blocker:* " + blocker},
				{"type": "mrkdwn", "text": "*Status:* " + db.DerivedStatus(req)},
				{"type": "mrkdwn", "text": "*Queue:* " + queueLabel},
				{"type": "mrkdwn", "text": "*Type:* " + req.RequestType},
				{"type": "mrkdwn", "text": "*Priority:* " + req.Priority},
				{"type": "mrkdwn", "text": "*Due:* " + due},
				{"type": "mrkdwn", "text": "*State:* " + req.State},
			},
		},
		{
			"type":     "actions",
			"elements": actions,
		},
	}
	if !isClosed {
		blocks = append(blocks, map[string]any{
			"type": "actions",
			"elements": []map[string]any{
				button("Assign to me", "tg_assign_me", req.ID.String(), "", false),
				button("Waiting on requester", "tg_waiting_requester", req.ID.String(), "", false),
				button("Snooze till tomorrow", "tg_snooze_tomorrow", req.ID.String(), "", false),
			},
		})
	}
	if trackerText != "" {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{"type": "mrkdwn", "text": trackerText},
		})
	}
	return blocks
}

func RenderTriageModal(req db.Request, queues []db.Queue) map[string]any {
	requestID := req.ID.String()
	blocks := []map[string]any{
		checkboxInput("ack_block", "ack_checkbox", "Acknowledge", []map[string]any{
			{"text": map[string]any{"type": "plain_text", "text": "Mark as acknowledged"}, "value": "ack"},
		}, []string{"ack"}),
		{
			"type":     "input",
			"block_id": "owner_block",
			"optional": true,
			"label":    map[string]any{"type": "plain_text", "text": "Owner"},
			"element": map[string]any{
				"type":        "users_select",
				"action_id":   "owner_select",
				"placeholder": map[string]any{"type": "plain_text", "text": "Select owner"},
			},
		},
		staticSelectInput("priority_block", "priority_select", "Priority", []string{"P0", "P1", "P2"}, req.Priority, false),
		staticSelectInput("type_block", "type_select", "Request type", []string{"bug", "incident", "access", "infra", "question", "task", "other"}, req.RequestType, false),
		queueSelectInput("queue_block", "queue_select", queues, req.QueueID),
		{
			"type":     "input",
			"block_id": "due_block",
			"optional": true,
			"label":    map[string]any{"type": "plain_text", "text": "Due date"},
			"element": map[string]any{
				"type":      "datepicker",
				"action_id": "due_picker",
			},
		},
		staticSelectInput("waiting_block", "waiting_select", "Waiting on", []string{"none", "requester", "other_team", "external_vendor", "scheduled_work"}, req.WaitingOn, true),
		{
			"type":     "input",
			"block_id": "snooze_block",
			"optional": true,
			"label":    map[string]any{"type": "plain_text", "text": "Snooze until"},
			"element": map[string]any{
				"type":      "datepicker",
				"action_id": "snooze_picker",
			},
		},
		checkboxInput("tracker_block", "tracker_checkbox", "Tracker", []map[string]any{
			{"text": map[string]any{"type": "plain_text", "text": "Link tracker issue"}, "value": "link_tracker"},
		}, nil),
	}

	if req.OwnerUserID != nil && *req.OwnerUserID != "" {
		blocks[1]["element"].(map[string]any)["initial_user"] = *req.OwnerUserID
	}
	if req.DueAt != nil {
		blocks[5]["element"].(map[string]any)["initial_date"] = req.DueAt.UTC().Format("2006-01-02")
	}
	if req.SnoozedUntil != nil {
		blocks[7]["element"].(map[string]any)["initial_date"] = req.SnoozedUntil.UTC().Format("2006-01-02")
	}

	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_triage_submit",
		"private_metadata": requestID,
		"title":            map[string]any{"type": "plain_text", "text": "Triage request"},
		"submit":           map[string]any{"type": "plain_text", "text": "Save"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks":           blocks,
	}
}

func blockerLabel(req db.Request) string {
	if req.SnoozedUntil != nil && req.SnoozedUntil.After(time.Now().UTC()) {
		return "Snoozed until " + req.SnoozedUntil.UTC().Format("2006-01-02 15:04 UTC")
	}
	if normalized := db.NormalizeWaitingOn(req.WaitingOn); normalized != db.WaitingOnNone {
		return strings.Title(strings.ReplaceAll(normalized, "_", " "))
	}
	return "Active"
}

func RenderWaitingModal(req db.Request) map[string]any {
	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_waiting_submit",
		"private_metadata": req.ID.String(),
		"title":            map[string]any{"type": "plain_text", "text": "Set waiting"},
		"submit":           map[string]any{"type": "plain_text", "text": "Save"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []map[string]any{
			staticSelectInput("waiting_block", "waiting_select", "Waiting on", []string{"requester", "other_team", "external_vendor", "scheduled_work"}, req.WaitingOn, false),
			{
				"type":     "input",
				"block_id": "comment_block",
				"optional": true,
				"label":    map[string]any{"type": "plain_text", "text": "Comment"},
				"element": map[string]any{
					"type":      "plain_text_input",
					"action_id": "comment_input",
					"multiline": true,
				},
			},
			{
				"type":     "input",
				"block_id": "review_block",
				"optional": true,
				"label":    map[string]any{"type": "plain_text", "text": "Review date"},
				"element": map[string]any{
					"type":      "datepicker",
					"action_id": "review_picker",
				},
			},
		},
	}
}

func RenderSnoozeModal(req db.Request) map[string]any {
	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_snooze_submit",
		"private_metadata": req.ID.String(),
		"title":            map[string]any{"type": "plain_text", "text": "Snooze request"},
		"submit":           map[string]any{"type": "plain_text", "text": "Snooze"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []map[string]any{
			staticSelectInput("preset_block", "preset_select", "Preset", []string{"4_hours", "tomorrow", "next_monday", "custom"}, "tomorrow", false),
			{
				"type":     "input",
				"block_id": "custom_block",
				"optional": true,
				"label":    map[string]any{"type": "plain_text", "text": "Custom date"},
				"element": map[string]any{
					"type":      "datepicker",
					"action_id": "custom_picker",
				},
			},
		},
	}
}

func RenderCloseModal(req db.Request) map[string]any {
	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_close_submit",
		"private_metadata": req.ID.String(),
		"title":            map[string]any{"type": "plain_text", "text": "Close request"},
		"submit":           map[string]any{"type": "plain_text", "text": "Close"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []map[string]any{
			staticSelectInput("close_block", "close_select", "Close reason", []string{"resolved", "duplicate", "not_planned", "noise", "invalid"}, "resolved", false),
		},
	}
}

func RenderSLAHint(req db.Request, clocks []db.RequestSLAClock, now time.Time) string {
	if strings.EqualFold(req.State, db.RequestStateClosed) {
		return "SLA tracking stopped"
	}
	if len(clocks) == 0 {
		return "SLA clocks pending"
	}
	parts := make([]string, 0, len(clocks))
	for _, clock := range clocks {
		label := strings.ToUpper(clock.ClockType)
		switch clock.State {
		case "satisfied":
			parts = append(parts, fmt.Sprintf("%s satisfied", label))
		case "paused":
			parts = append(parts, fmt.Sprintf("%s paused", label))
		case "breached":
			parts = append(parts, fmt.Sprintf("⚠️ %s breached", label))
		case "stopped":
			parts = append(parts, fmt.Sprintf("%s stopped", label))
		default:
			if clock.TargetAt != nil {
				remaining := clock.TargetAt.Sub(now)
				if remaining <= 0 {
					parts = append(parts, fmt.Sprintf("⚠️ %s due", label))
				} else {
					text := fmt.Sprintf("%s in %dm", label, ceilMinutes(remaining))
					if clockIsAtRisk(clock, now) {
						text = "At risk: " + text
					}
					parts = append(parts, text)
				}
			} else {
				parts = append(parts, fmt.Sprintf("%s running", label))
			}
		}
	}
	return strings.Join(parts, " • ")
}

func clockIsAtRisk(clock db.RequestSLAClock, now time.Time) bool {
	if clock.TargetAt == nil || clock.State != "running" {
		return false
	}
	total := clock.TargetAt.Sub(clock.StartedAt)
	if total <= 0 {
		return false
	}
	elapsed := now.Sub(clock.StartedAt)
	return elapsed >= time.Duration(float64(total)*0.8)
}

func ceilMinutes(d time.Duration) int {
	minutes := int(math.Ceil(d.Minutes()))
	if minutes < 1 {
		return 1
	}
	return minutes
}

func queueSelectInput(blockID, actionID string, queues []db.Queue, currentQueueID *uuid.UUID) map[string]any {
	options := make([]map[string]any, 0, len(queues))
	var initial map[string]any
	for _, queue := range queues {
		option := map[string]any{
			"text":  map[string]any{"type": "plain_text", "text": queue.Name},
			"value": queue.ID.String(),
		}
		options = append(options, option)
		if currentQueueID != nil && queue.ID == *currentQueueID {
			initial = option
		}
	}
	element := map[string]any{
		"type":        "static_select",
		"action_id":   actionID,
		"placeholder": map[string]any{"type": "plain_text", "text": "Select queue"},
		"options":     options,
	}
	if initial != nil {
		element["initial_option"] = initial
	}
	return map[string]any{
		"type":     "input",
		"block_id": blockID,
		"label":    map[string]any{"type": "plain_text", "text": "Queue"},
		"element":  element,
	}
}

func staticSelectInput(blockID, actionID, label string, values []string, current string, optional bool) map[string]any {
	options := make([]map[string]any, 0, len(values))
	var initial map[string]any
	for _, value := range values {
		option := map[string]any{
			"text":  map[string]any{"type": "plain_text", "text": renderOptionLabel(value)},
			"value": value,
		}
		options = append(options, option)
		if strings.EqualFold(strings.TrimSpace(current), strings.TrimSpace(value)) {
			initial = option
		}
	}
	element := map[string]any{
		"type":        "static_select",
		"action_id":   actionID,
		"placeholder": map[string]any{"type": "plain_text", "text": label},
		"options":     options,
	}
	if initial != nil {
		element["initial_option"] = initial
	}
	return map[string]any{
		"type":     "input",
		"block_id": blockID,
		"optional": optional,
		"label":    map[string]any{"type": "plain_text", "text": label},
		"element":  element,
	}
}

func checkboxInput(blockID, actionID, label string, options []map[string]any, initial []string) map[string]any {
	element := map[string]any{
		"type":      "checkboxes",
		"action_id": actionID,
		"options":   options,
	}
	if len(initial) > 0 {
		initialOptions := make([]map[string]any, 0, len(options))
		for _, option := range options {
			if value, _ := option["value"].(string); containsString(initial, value) {
				initialOptions = append(initialOptions, option)
			}
		}
		element["initial_options"] = initialOptions
	}
	return map[string]any{
		"type":     "input",
		"block_id": blockID,
		"optional": true,
		"label":    map[string]any{"type": "plain_text", "text": label},
		"element":  element,
	}
}

func overflowMenu(req db.Request) map[string]any {
	options := []map[string]any{
		overflowOption("Set waiting", "waiting|"+req.ID.String()),
		overflowOption("Snooze", "snooze|"+req.ID.String()),
		overflowOption("Reassign", "reassign|"+req.ID.String()),
		overflowOption("Link tracker issue", "tracker|"+req.ID.String()),
		overflowOption("Close request", "close|"+req.ID.String()),
	}
	if strings.EqualFold(req.State, db.RequestStateClosed) {
		options = []map[string]any{
			overflowOption("Reopen", "reopen|"+req.ID.String()),
			overflowOption("Link tracker issue", "tracker|"+req.ID.String()),
		}
	}
	return map[string]any{
		"type":      "overflow",
		"action_id": "tg_more",
		"options":   options,
	}
}

func overflowOption(text, value string) map[string]any {
	return map[string]any{
		"text":  map[string]any{"type": "plain_text", "text": text},
		"value": value,
	}
}

func button(text, actionID, value, style string, disabled bool) map[string]any {
	m := map[string]any{
		"type":      "button",
		"text":      map[string]any{"type": "plain_text", "text": text},
		"action_id": actionID,
		"value":     value,
	}
	if style != "" {
		m["style"] = style
	}
	if disabled {
		m["confirm"] = map[string]any{
			"title":   map[string]any{"type": "plain_text", "text": "Closed"},
			"text":    map[string]any{"type": "mrkdwn", "text": "Request already closed."},
			"confirm": map[string]any{"type": "plain_text", "text": "OK"},
			"deny":    map[string]any{"type": "plain_text", "text": "Cancel"},
		}
	}
	return m
}

func renderOptionLabel(value string) string {
	switch strings.TrimSpace(value) {
	case "other_team":
		return "Other team"
	case "external_vendor":
		return "External vendor"
	case "scheduled_work":
		return "Scheduled work"
	case "not_planned":
		return "Not planned"
	case "4_hours":
		return "4 hours"
	case "next_monday":
		return "Next Monday"
	default:
		if strings.Contains(value, "_") {
			return strings.Title(strings.ReplaceAll(value, "_", " "))
		}
		return strings.Title(value)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
