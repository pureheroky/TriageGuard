package db

import (
	"strings"
	"testing"
)

func TestOpsOutboxQueriesQualifyStatusWithAliases(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		want     []string
		dontWant []string
	}{
		{
			name:  "workspace ops summary query",
			query: workspaceOpsSummaryOutboxQuery,
			want: []string{
				"select kind, o.status, o.available_at, o.failed_at",
				"and o.status in ('retrying', 'failed', 'pending', 'processing')",
			},
			dontWant: []string{
				"select kind, status, available_at, failed_at",
				"and status in ('retrying', 'failed', 'pending', 'processing')",
			},
		},
		{
			name:  "internal ops summary query",
			query: internalOpsSummaryOutboxQuery,
			want: []string{
				"count(*) filter (where o.status = 'failed')::int",
				"count(*) filter (where o.status in ('retrying', 'pending', 'processing'))::int",
				"min(o.available_at)",
			},
			dontWant: []string{
				"count(*) filter (where status = 'failed')::int",
				"count(*) filter (where status in ('retrying', 'pending', 'processing'))::int",
				"min(available_at)",
			},
		},
		{
			name:  "workspace sync lag query",
			query: workspaceOpsSyncLagQuery,
			want: []string{
				"coalesce(ei.last_sync_at, ei.created_at)",
				"join requests r on r.id = ei.request_id",
				"where r.workspace_id = $1",
			},
			dontWant: []string{
				"coalesce(last_sync_at, created_at)",
				"coalesce(ei.last_sync_at, created_at)",
				"where workspace_id = $1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := strings.Join(strings.Fields(tt.query), " ")
			for _, want := range tt.want {
				if !strings.Contains(query, want) {
					t.Fatalf("expected query to contain %q\nquery=%s", want, query)
				}
			}
			for _, dontWant := range tt.dontWant {
				if strings.Contains(query, dontWant) {
					t.Fatalf("query must not contain ambiguous fragment %q\nquery=%s", dontWant, query)
				}
			}
		})
	}
}

func TestWorkspaceOpsHealthBanner(t *testing.T) {
	tests := []struct {
		name    string
		summary WorkspaceOpsSummary
		want    string
	}{
		{
			name:    "slack disconnected has top priority",
			summary: WorkspaceOpsSummary{},
			want:    "Slack is not connected. Queue control is disabled until Slack is reconnected.",
		},
		{
			name: "attention banner covers failed side effects",
			summary: WorkspaceOpsSummary{
				SlackConnected:     true,
				DeadLetterCount:    2,
				FailedEscalations:  1,
				FailedTrackerSyncs: 3,
				RetryBacklogCount:  9,
			},
			want: "Attention needed: 2 dead letters, 1 failed escalations, 3 failed tracker syncs in the last 7 days.",
		},
		{
			name: "backlog banner used when failures are clear",
			summary: WorkspaceOpsSummary{
				SlackConnected:    true,
				RetryBacklogCount: 4,
			},
			want: "Processing backlog: 4 pending or retrying side effects.",
		},
		{
			name: "healthy banner for clean workspace",
			summary: WorkspaceOpsSummary{
				SlackConnected: true,
			},
			want: "All integrations and queue-control side effects are healthy.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workspaceOpsHealthBanner(tt.summary)
			if got != tt.want {
				t.Fatalf("workspaceOpsHealthBanner() = %q want %q", got, tt.want)
			}
		})
	}
}
