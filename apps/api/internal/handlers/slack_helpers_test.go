package handlers

import (
	"testing"
	"time"

	"triageguard/apps/api/internal/db"
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

func TestNextBusinessStartForPolicy(t *testing.T) {
	start := "09:30:00"
	tests := []struct {
		name   string
		policy db.QueuePolicy
		now    time.Time
		want   time.Time
	}{
		{
			name: "business hours skip weekend and use queue start time",
			policy: db.QueuePolicy{
				Timezone:             "UTC",
				BusinessHoursEnabled: true,
				BusinessHoursStart:   &start,
				BusinessDaysMask:     (1 << int(time.Monday)) | (1 << int(time.Tuesday)) | (1 << int(time.Wednesday)) | (1 << int(time.Thursday)) | (1 << int(time.Friday)),
			},
			now:  time.Date(2026, time.March, 27, 17, 0, 0, 0, time.UTC),
			want: time.Date(2026, time.March, 30, 9, 30, 0, 0, time.UTC),
		},
		{
			name: "disabled business hours defaults to next day 09:00 in queue timezone",
			policy: db.QueuePolicy{
				Timezone: "UTC",
			},
			now:  time.Date(2026, time.March, 31, 18, 15, 0, 0, time.UTC),
			want: time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		},
		{
			name: "invalid timezone falls back to utc",
			policy: db.QueuePolicy{
				Timezone: "Invalid/Timezone",
			},
			now:  time.Date(2026, time.March, 31, 23, 0, 0, 0, time.UTC),
			want: time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextBusinessStartForPolicy(tt.policy, tt.now)
			if got == nil {
				t.Fatalf("expected non-nil next business start")
			}
			if !got.Equal(tt.want) {
				t.Fatalf("nextBusinessStartForPolicy() = %s want %s", got.UTC().Format(time.RFC3339), tt.want.UTC().Format(time.RFC3339))
			}
		})
	}
}
