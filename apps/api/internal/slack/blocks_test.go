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
		ID:        uuid.New(),
		Status:    "NEW",
		Priority:  "P2",
		CreatedAt: time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "", "UTC", 15, 30)
	ids := collectActionIDs(blocks)

	if !ids["tg_resolve"] {
		t.Fatalf("expected tg_resolve action in triage blocks")
	}
	if ids["tg_reopen"] {
		t.Fatalf("did not expect tg_reopen action for non-resolved request")
	}
}

func TestRenderTriageBlocksShowsReopenForResolved(t *testing.T) {
	req := db.Request{
		ID:        uuid.New(),
		Status:    "RESOLVED",
		Priority:  "P2",
		CreatedAt: time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "", "UTC", 15, 30)
	ids := collectActionIDs(blocks)

	if !ids["tg_reopen"] {
		t.Fatalf("expected tg_reopen action for resolved request")
	}
	if ids["tg_resolve"] {
		t.Fatalf("did not expect tg_resolve action for resolved request")
	}
}

func TestRenderTriageBlocksShowsReopenForIgnored(t *testing.T) {
	req := db.Request{
		ID:        uuid.New(),
		Status:    "IGNORED",
		Priority:  "P2",
		CreatedAt: time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "", "UTC", 15, 30)
	ids := collectActionIDs(blocks)

	if !ids["tg_reopen"] {
		t.Fatalf("expected tg_reopen action for ignored request")
	}
	if ids["tg_resolve"] {
		t.Fatalf("did not expect tg_resolve action for ignored request")
	}
}

func TestRenderTriageBlocksIgnoreHasIrreversibleConfirm(t *testing.T) {
	req := db.Request{
		ID:        uuid.New(),
		Status:    "NEW",
		Priority:  "P2",
		CreatedAt: time.Now().UTC(),
	}

	blocks := RenderTriageBlocks(req, "", "UTC", 15, 30)
	ignoreAction, ok := findActionByID(blocks, "tg_ignore")
	if !ok {
		t.Fatalf("expected tg_ignore action in triage blocks")
	}

	confirm, _ := ignoreAction["confirm"].(map[string]any)
	if len(confirm) == 0 {
		t.Fatalf("expected confirm on tg_ignore action")
	}
	textObj, _ := confirm["text"].(map[string]any)
	text, _ := textObj["text"].(string)
	if !strings.Contains(strings.ToLower(text), "irreversible") {
		t.Fatalf("expected tg_ignore confirm text to mention irreversible action, got %q", text)
	}
}

func TestRenderSLAHint(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name string
		req  db.Request
		want string
	}{
		{
			name: "new request shows ack due",
			req: db.Request{
				Status:    "NEW",
				CreatedAt: now.Add(-5 * time.Minute),
			},
			want: "Ack due in",
		},
		{
			name: "acked unassigned shows assign due",
			req: db.Request{
				Status:    "ACKED",
				CreatedAt: now.Add(-10 * time.Minute),
			},
			want: "Assign due in",
		},
		{
			name: "ignored shows paused",
			req: db.Request{
				Status:    "IGNORED",
				CreatedAt: now.Add(-60 * time.Minute),
			},
			want: "SLA tracking paused",
		},
		{
			name: "ack overdue flag",
			req: db.Request{
				Status:           "NEW",
				CreatedAt:        now.Add(-60 * time.Minute),
				AckOverdueSentAt: ptrTime(now.Add(-1 * time.Minute)),
			},
			want: "Ack overdue",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderSLAHint(tt.req, 15, 30, now)
			if !strings.Contains(got, tt.want) {
				t.Fatalf("renderSLAHint = %q, expected to contain %q", got, tt.want)
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
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
