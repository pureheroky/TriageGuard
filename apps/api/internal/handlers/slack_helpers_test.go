package handlers

import (
	"testing"

	"triageguard/apps/api/internal/linear"
)

func TestLinearStateTypeForRequestStatus(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{status: "NEW", want: "unstarted"},
		{status: "ACKED", want: "unstarted"},
		{status: "ASSIGNED", want: "started"},
		{status: "IN_PROGRESS", want: "started"},
		{status: "RESOLVED", want: "completed"},
		{status: "IGNORED", want: "canceled"},
	}

	for _, tt := range tests {
		got := linearStateTypeForRequestStatus(tt.status)
		if got != tt.want {
			t.Fatalf("linearStateTypeForRequestStatus(%q) = %q want %q", tt.status, got, tt.want)
		}
	}
}

func TestPickLinearStateID(t *testing.T) {
	states := []linear.WorkflowState{
		{ID: "backlog", Type: "backlog"},
		{ID: "todo", Type: "unstarted"},
		{ID: "doing", Type: "started"},
		{ID: "done", Type: "completed"},
	}

	if got := pickLinearStateID(states, "started"); got != "doing" {
		t.Fatalf("started selection = %q", got)
	}
	if got := pickLinearStateID(states, "completed"); got != "done" {
		t.Fatalf("completed selection = %q", got)
	}
	if got := pickLinearStateID(states, "unknown"); got == "" {
		t.Fatalf("unknown desired type should still return a fallback state")
	}
	if got := pickLinearStateID(nil, "started"); got != "" {
		t.Fatalf("empty states must return empty id")
	}
}

func TestSlackActionDedupKey(t *testing.T) {
	blockPayload := slackActionPayload{
		Type: "block_actions",
		Team: struct {
			ID string `json:"id"`
		}{ID: "T1"},
		User: struct {
			ID string `json:"id"`
		}{ID: "U1"},
		Container: struct {
			ChannelID string `json:"channel_id"`
			MessageTS string `json:"message_ts"`
		}{ChannelID: "C1", MessageTS: "1710000000.000100"},
		Actions: []struct {
			ActionID       string `json:"action_id"`
			Value          string `json:"value"`
			ActionTS       string `json:"action_ts"`
			SelectedOption struct {
				Value string `json:"value"`
			} `json:"selected_option"`
		}{
			{
				ActionID: "tg_ack",
				Value:    "request-id",
				ActionTS: "1710000001.000200",
			},
		},
	}

	key1 := slackActionDedupKey(blockPayload)
	key2 := slackActionDedupKey(blockPayload)
	if key1 == "" || key2 == "" {
		t.Fatalf("dedup key must be non-empty")
	}
	if key1 != key2 {
		t.Fatalf("dedup key must be stable for same payload")
	}
}
