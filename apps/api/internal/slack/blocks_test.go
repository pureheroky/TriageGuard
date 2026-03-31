package slack

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
)

func TestRenderTriageBlocksIncludesResolveAction(t *testing.T) {
	req := db.Request{
		ID:          uuid.New(),
		State:       db.RequestStateOpen,
		RequestType: "other",
		Priority:    "P2",
		CreatedAt:   time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "Default queue", "", "ACK in 10m")
	ids := collectActionIDs(blocks)

	if !ids["tg_resolve"] {
		t.Fatalf("expected tg_resolve action in triage blocks")
	}
	if !ids["tg_assign_me"] || !ids["tg_waiting_requester"] || !ids["tg_snooze_tomorrow"] {
		t.Fatalf("expected hot actions in triage blocks for open request")
	}
	if ids["tg_reopen"] {
		t.Fatalf("did not expect tg_reopen action for non-resolved request")
	}
}

func TestRenderTriageBlocksShowsReopenForResolved(t *testing.T) {
	req := db.Request{
		ID:           uuid.New(),
		State:        db.RequestStateClosed,
		ClosedReason: strPtr(db.ClosedReasonResolved),
		RequestType:  "other",
		Priority:     "P2",
		CreatedAt:    time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "Default queue", "", "SLA stopped")
	ids := collectActionIDs(blocks)

	if !ids["tg_reopen"] {
		t.Fatalf("expected tg_reopen action for resolved request")
	}
	if ids["tg_resolve"] {
		t.Fatalf("did not expect tg_resolve action for resolved request")
	}
	if ids["tg_assign_me"] || ids["tg_waiting_requester"] || ids["tg_snooze_tomorrow"] {
		t.Fatalf("did not expect hot actions for closed request")
	}
}

func TestRenderTriageBlocksShowsReopenForIgnored(t *testing.T) {
	req := db.Request{
		ID:           uuid.New(),
		State:        db.RequestStateClosed,
		ClosedReason: strPtr(db.ClosedReasonNoise),
		RequestType:  "other",
		Priority:     "P2",
		CreatedAt:    time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "Default queue", "", "SLA stopped")
	ids := collectActionIDs(blocks)

	if !ids["tg_reopen"] {
		t.Fatalf("expected tg_reopen action for ignored request")
	}
	if ids["tg_resolve"] {
		t.Fatalf("did not expect tg_resolve action for ignored request")
	}
}

func TestRenderSLAHint(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name   string
		req    db.Request
		clocks []db.RequestSLAClock
		want   string
	}{
		{
			name: "new request shows ack due",
			req: db.Request{
				State:     db.RequestStateOpen,
				CreatedAt: now.Add(-5 * time.Minute),
			},
			clocks: []db.RequestSLAClock{{
				ClockType: "ack",
				StartedAt: now.Add(-5 * time.Minute),
				TargetAt:  ptrTime(now.Add(10 * time.Minute)),
				State:     "running",
			}},
			want: "ACK in",
		},
		{
			name: "acked unassigned shows assign due",
			req: db.Request{
				State:          db.RequestStateOpen,
				AcknowledgedAt: ptrTime(now.Add(-12 * time.Minute)),
				CreatedAt:      now.Add(-12 * time.Minute),
			},
			clocks: []db.RequestSLAClock{{
				ClockType: "assign",
				StartedAt: now.Add(-12 * time.Minute),
				TargetAt:  ptrTime(now.Add(3 * time.Minute)),
				State:     "running",
			}},
			want: "At risk: ASSIGN in",
		},
		{
			name: "closed shows stopped",
			req: db.Request{
				State:     db.RequestStateClosed,
				CreatedAt: now.Add(-60 * time.Minute),
			},
			want: "SLA tracking stopped",
		},
		{
			name: "breached clock",
			req: db.Request{
				State:     db.RequestStateOpen,
				CreatedAt: now.Add(-60 * time.Minute),
			},
			clocks: []db.RequestSLAClock{{
				ClockType:  "ack",
				StartedAt:  now.Add(-60 * time.Minute),
				TargetAt:   ptrTime(now.Add(-1 * time.Minute)),
				BreachedAt: ptrTime(now.Add(-1 * time.Minute)),
				State:      "breached",
			}},
			want: "breached",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RenderSLAHint(tt.req, tt.clocks, now)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("RenderSLAHint = %q, expected to contain %q", got, tt.want)
			}
		})
	}
}

func TestRenderTriageBlocksShowsSnoozedBlocker(t *testing.T) {
	snoozedUntil := time.Now().UTC().Add(2 * time.Hour)
	req := db.Request{
		ID:           uuid.New(),
		State:        db.RequestStateOpen,
		RequestType:  "incident",
		Priority:     "P1",
		SnoozedUntil: &snoozedUntil,
		CreatedAt:    time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "Platform requests", "", "ACK in 10m")
	if !sectionContains(blocks, "Blocker:* Snoozed until") {
		t.Fatalf("expected blocker section to show snoozed label")
	}
}

func TestRenderTriageModalIncludesQueueWaitingAndSnoozeInputs(t *testing.T) {
	queueID := uuid.New()
	req := db.Request{
		ID:          uuid.New(),
		State:       db.RequestStateOpen,
		RequestType: "task",
		Priority:    "P2",
		QueueID:     &queueID,
		CreatedAt:   time.Now().UTC(),
	}
	queues := []db.Queue{{ID: queueID, Name: "Platform requests"}}

	modal := RenderTriageModal(req, queues)
	blocks, ok := modal["blocks"].([]map[string]any)
	if !ok {
		t.Fatalf("expected blocks array in triage modal")
	}
	if !containsInputBlock(blocks, "queue_block") {
		t.Fatalf("expected queue selector in triage modal")
	}
	if !containsInputBlock(blocks, "waiting_block") {
		t.Fatalf("expected waiting selector in triage modal")
	}
	if !containsInputBlock(blocks, "snooze_block") {
		t.Fatalf("expected snooze selector in triage modal")
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}

func strPtr(v string) *string {
	return &v
}

func collectActionIDs(blocks []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, block := range blocks {
		if block["type"] != "actions" {
			continue
		}
		elements, _ := block["elements"].([]map[string]any)
		for _, el := range elements {
			actionID, _ := el["action_id"].(string)
			if actionID == "" {
				continue
			}
			out[actionID] = true
		}
	}
	return out
}

func findActionByID(blocks []map[string]any, actionID string) (map[string]any, bool) {
	for _, block := range blocks {
		if block["type"] != "actions" {
			continue
		}
		elements, _ := block["elements"].([]map[string]any)
		for _, el := range elements {
			id, _ := el["action_id"].(string)
			if id == actionID {
				return el, true
			}
		}
	}
	return nil, false
}

func containsInputBlock(blocks []map[string]any, blockID string) bool {
	for _, block := range blocks {
		if block["type"] != "input" {
			continue
		}
		id, _ := block["block_id"].(string)
		if id == blockID {
			return true
		}
	}
	return false
}

func sectionContains(blocks []map[string]any, needle string) bool {
	for _, block := range blocks {
		if block["type"] == "section" {
			if text, ok := block["text"].(map[string]any); ok {
				if value, _ := text["text"].(string); strings.Contains(value, needle) {
					return true
				}
			}
			if fields, ok := block["fields"].([]map[string]any); ok {
				for _, field := range fields {
					if value, _ := field["text"].(string); strings.Contains(value, needle) {
						return true
					}
				}
			}
		}
	}
	return false
}
