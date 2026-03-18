package slack

import (
	"fmt"
	"math"
	"strings"
	"time"

	"triageguard/apps/api/internal/db"
)

func RenderTriageBlocks(req db.Request, linearURL string, timezone string, ackSLAMinutes int, assignSLAMinutes int) []map[string]any {
	status := req.Status
	owner := "Unassigned"
	if req.OwnerSlackID != nil && *req.OwnerSlackID != "" {
		owner = "<@" + *req.OwnerSlackID + ">"
	}
	due := "—"
	if req.DueAt != nil {
		loc, err := time.LoadLocation(timezone)
		if err != nil {
			loc = time.UTC
		}
		due = req.DueAt.In(loc).Format("2006-01-02")
	}
	slaHint := renderSLAHint(req, ackSLAMinutes, assignSLAMinutes, time.Now().UTC())
	linearText := ""
	if linearURL != "" {
		parts := strings.Split(strings.TrimPrefix(linearURL, "https://"), "/")
		label := "Issue"
		if len(parts) > 0 {
			label = parts[len(parts)-1]
		}
		linearText = fmt.Sprintf("*Linear:* <%s|%s>", linearURL, label)
	}
	isIgnored := req.Status == "IGNORED"
	isResolved := req.Status == "RESOLVED"
	isClosed := isIgnored || isResolved

	resolveButton := button("Resolve", "tg_resolve", req.ID.String(), "", isClosed)
	if isResolved || isIgnored {
		resolveButton = button("Reopen", "tg_reopen", req.ID.String(), "primary", false)
	}
	ignoreButton := button("Ignore", "tg_ignore", req.ID.String(), "danger", isClosed)
	if !isClosed {
		ignoreButton["confirm"] = irreversibleIgnoreConfirm()
	}

	actions := []map[string]any{
		button("Acknowledge", "tg_ack", req.ID.String(), "primary", isClosed),
		button("Assign", "tg_assign", req.ID.String(), "", isClosed),
		prioritySelect(req.ID.String(), req.Priority, isClosed),
		button("Due date", "tg_due", req.ID.String(), "", isClosed),
		resolveButton,
		button("Convert to Linear", "tg_convert", req.ID.String(), "", isClosed),
		ignoreButton,
	}

	blocks := []map[string]any{
		{
			"type": "header",
			"text": map[string]any{"type": "plain_text", "text": "TriageGuard Request", "emoji": true},
		},
		{
			"type": "section",
			"fields": []map[string]any{
				{"type": "mrkdwn", "text": "*Status:* " + status},
				{"type": "mrkdwn", "text": "*Priority:* " + req.Priority},
				{"type": "mrkdwn", "text": "*Owner:* " + owner},
				{"type": "mrkdwn", "text": "*Due:* " + due},
			},
		},
		{
			"type":     "context",
			"elements": []map[string]any{{"type": "mrkdwn", "text": slaHint}},
		},
		{
			"type":     "actions",
			"elements": actions,
		},
	}
	if linearText != "" {
		blocks = append(blocks, map[string]any{
			"type": "section",
			"text": map[string]any{"type": "mrkdwn", "text": linearText},
		})
	}
	return blocks
}

func renderSLAHint(req db.Request, ackSLAMinutes int, assignSLAMinutes int, now time.Time) string {
	status := strings.ToUpper(strings.TrimSpace(req.Status))
	if status == "IGNORED" || status == "RESOLVED" {
		return "SLA tracking paused"
	}
	if req.AckOverdueSentAt != nil {
		return "⚠️ Ack overdue"
	}
	if req.AssignOverdueSentAt != nil {
		return "⚠️ Assign overdue"
	}
	if ackSLAMinutes <= 0 {
		ackSLAMinutes = 15
	}
	if assignSLAMinutes <= 0 {
		assignSLAMinutes = 30
	}

	if status == "NEW" {
		deadline := req.CreatedAt.Add(time.Duration(ackSLAMinutes) * time.Minute)
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return "⚠️ Ack overdue"
		}
		return fmt.Sprintf("SLA: Ack due in %dm", ceilMinutes(remaining))
	}
	if (status == "NEW" || status == "ACKED") && (req.OwnerSlackID == nil || strings.TrimSpace(*req.OwnerSlackID) == "") {
		deadline := req.CreatedAt.Add(time.Duration(assignSLAMinutes) * time.Minute)
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return "⚠️ Assign overdue"
		}
		return fmt.Sprintf("SLA: Assign due in %dm", ceilMinutes(remaining))
	}

	return "SLA tracking active"
}

func ceilMinutes(d time.Duration) int {
	minutes := int(math.Ceil(d.Minutes()))
	if minutes < 1 {
		return 1
	}
	return minutes
}

func RenderAssignModal(requestID string) map[string]any {
	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_assign_submit",
		"private_metadata": requestID,
		"title":            map[string]any{"type": "plain_text", "text": "Assign owner"},
		"submit":           map[string]any{"type": "plain_text", "text": "Assign"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []map[string]any{
			{
				"type":     "input",
				"block_id": "owner_block",
				"label":    map[string]any{"type": "plain_text", "text": "Owner"},
				"element":  map[string]any{"type": "users_select", "action_id": "owner_select"},
			},
		},
	}
}

func RenderDueModal(requestID string) map[string]any {
	return map[string]any{
		"type":             "modal",
		"callback_id":      "tg_due_submit",
		"private_metadata": requestID,
		"title":            map[string]any{"type": "plain_text", "text": "Set due date"},
		"submit":           map[string]any{"type": "plain_text", "text": "Save"},
		"close":            map[string]any{"type": "plain_text", "text": "Cancel"},
		"blocks": []map[string]any{
			{
				"type":     "input",
				"block_id": "due_block",
				"label":    map[string]any{"type": "plain_text", "text": "Due date"},
				"element":  map[string]any{"type": "datepicker", "action_id": "due_picker"},
			},
		},
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

func irreversibleIgnoreConfirm() map[string]any {
	return map[string]any{
		"title":   map[string]any{"type": "plain_text", "text": "Ignore request?"},
		"text":    map[string]any{"type": "mrkdwn", "text": "This action is irreversible. The request will be marked as ignored and SLA reminders will stop."},
		"confirm": map[string]any{"type": "plain_text", "text": "Ignore"},
		"deny":    map[string]any{"type": "plain_text", "text": "Cancel"},
	}
}

func prioritySelect(requestID, current string, disabled bool) map[string]any {
	selectEl := map[string]any{
		"type":        "static_select",
		"action_id":   "tg_priority",
		"placeholder": map[string]any{"type": "plain_text", "text": "Priority"},
		"options": []map[string]any{
			{"text": map[string]any{"type": "plain_text", "text": "P0"}, "value": "P0|" + requestID},
			{"text": map[string]any{"type": "plain_text", "text": "P1"}, "value": "P1|" + requestID},
			{"text": map[string]any{"type": "plain_text", "text": "P2"}, "value": "P2|" + requestID},
		},
	}
	if current != "" {
		selectEl["initial_option"] = map[string]any{"text": map[string]any{"type": "plain_text", "text": current}, "value": current + "|" + requestID}
	}
	if disabled {
		selectEl["confirm"] = map[string]any{
			"title":   map[string]any{"type": "plain_text", "text": "Closed"},
			"text":    map[string]any{"type": "mrkdwn", "text": "Request already closed."},
			"confirm": map[string]any{"type": "plain_text", "text": "OK"},
			"deny":    map[string]any{"type": "plain_text", "text": "Cancel"},
		}
	}
	return selectEl
}
