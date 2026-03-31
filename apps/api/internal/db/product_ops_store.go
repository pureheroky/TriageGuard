package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const workspaceOpsSummaryOutboxQuery = `
		select kind, o.status, o.available_at, o.failed_at
		from notification_outbox o
		join requests r on r.id = o.request_id
		where r.workspace_id = $1
		  and o.status in ('retrying', 'failed', 'pending', 'processing')
	`

const internalOpsSummaryOutboxQuery = `
		select
			count(*) filter (where o.status = 'failed')::int,
			count(*) filter (where o.status in ('retrying', 'pending', 'processing'))::int,
			floor(extract(epoch from (now() - min(o.available_at))) / 60)::int,
			count(distinct r.workspace_id)::int
		from notification_outbox o
		left join requests r on r.id = o.request_id
		where o.status in ('failed', 'retrying', 'pending', 'processing')
	`

const workspaceOpsSyncLagQuery = `
		select floor(extract(epoch from (now() - min(coalesce(ei.last_sync_at, ei.created_at)))) / 60)::int
		from external_issues ei
		join requests r on r.id = ei.request_id
		where r.workspace_id = $1
		  and (ei.last_sync_error is not null or ei.sync_state <> 'synced')
	`

func (s *Store) RecordJobRuntimeStart(ctx context.Context, jobName string, startedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		insert into job_runtime_status (job_name, last_started_at, updated_at)
		values ($1, $2, now())
		on conflict (job_name) do update set
			last_started_at = excluded.last_started_at,
			updated_at = now()
	`, strings.TrimSpace(jobName), startedAt)
	return err
}

func (s *Store) RecordJobRuntimeFinish(ctx context.Context, jobName string, finishedAt time.Time, duration time.Duration, runErr error) error {
	var errText *string
	var successAt *time.Time
	durationMS := int(duration.Milliseconds())
	if runErr != nil {
		value := strings.TrimSpace(runErr.Error())
		errText = &value
	} else {
		successAt = &finishedAt
	}
	_, err := s.pool.Exec(ctx, `
		insert into job_runtime_status (
			job_name, last_finished_at, last_success_at, last_error, last_duration_ms, updated_at
		) values ($1, $2, $3, $4, $5, now())
		on conflict (job_name) do update set
			last_finished_at = excluded.last_finished_at,
			last_success_at = coalesce(excluded.last_success_at, job_runtime_status.last_success_at),
			last_error = excluded.last_error,
			last_duration_ms = excluded.last_duration_ms,
			updated_at = now()
	`, strings.TrimSpace(jobName), finishedAt, successAt, errText, durationMS)
	return err
}

func (s *Store) ListJobRuntimeStatuses(ctx context.Context) ([]JobRuntimeStatus, error) {
	rows, err := s.pool.Query(ctx, `
		select job_name, last_started_at, last_finished_at, last_success_at, last_error, last_duration_ms, updated_at
		from job_runtime_status
		order by job_name asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]JobRuntimeStatus, 0)
	for rows.Next() {
		var item JobRuntimeStatus
		if err := rows.Scan(
			&item.JobName,
			&item.LastStartedAt,
			&item.LastFinishedAt,
			&item.LastSuccessAt,
			&item.LastError,
			&item.LastDurationMS,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListDeadLettersAll(ctx context.Context, limit int) ([]DeadLetter, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		select id, source, operation, workspace_id, request_id, external_id, error_message, payload, created_at
		from dead_letters
		order by created_at desc
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DeadLetter, 0, limit)
	for rows.Next() {
		var item DeadLetter
		if err := rows.Scan(
			&item.ID,
			&item.Source,
			&item.Operation,
			&item.WorkspaceID,
			&item.RequestID,
			&item.ExternalID,
			&item.ErrorMessage,
			&item.Payload,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListWorkspaceOutboxItems(ctx context.Context, workspaceID uuid.UUID, limit int) ([]NotificationOutboxItem, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		select o.id, o.request_id, o.event_id, o.kind, o.destination_type, o.destination_value,
			o.payload_json, o.dedupe_key, o.status, o.retry_count, o.available_at, o.sent_at,
			o.failed_at, o.last_error, o.created_at, o.updated_at
		from notification_outbox o
		join requests r on r.id = o.request_id
		where r.workspace_id = $1
		  and o.status in ('retrying', 'failed', 'processing', 'pending')
		order by o.updated_at desc, o.created_at desc
		limit $2
	`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NotificationOutboxItem, 0, limit)
	for rows.Next() {
		var item NotificationOutboxItem
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.EventID,
			&item.Kind,
			&item.DestinationType,
			&item.DestinationValue,
			&item.Payload,
			&item.DedupeKey,
			&item.Status,
			&item.RetryCount,
			&item.AvailableAt,
			&item.SentAt,
			&item.FailedAt,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListOutboxItemsAll(ctx context.Context, limit int) ([]NotificationOutboxItem, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		select id, request_id, event_id, kind, destination_type, destination_value,
			payload_json, dedupe_key, status, retry_count, available_at, sent_at,
			failed_at, last_error, created_at, updated_at
		from notification_outbox
		where status in ('retrying', 'failed', 'processing', 'pending')
		order by updated_at desc, created_at desc
		limit $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]NotificationOutboxItem, 0, limit)
	for rows.Next() {
		var item NotificationOutboxItem
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.EventID,
			&item.Kind,
			&item.DestinationType,
			&item.DestinationValue,
			&item.Payload,
			&item.DedupeKey,
			&item.Status,
			&item.RetryCount,
			&item.AvailableAt,
			&item.SentAt,
			&item.FailedAt,
			&item.LastError,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) WorkspaceOpsSummary(ctx context.Context, workspaceID uuid.UUID) (WorkspaceOpsSummary, error) {
	var summary WorkspaceOpsSummary
	runners, err := s.ListJobRuntimeStatuses(ctx)
	if err != nil {
		return WorkspaceOpsSummary{}, err
	}
	summary.Runners = runners

	_, slackErr := s.GetSlackInstallationByWorkspace(ctx, workspaceID)
	summary.SlackConnected = slackErr == nil
	_, linearErr := s.GetLinearInstallation(ctx, workspaceID)
	summary.LinearConnected = linearErr == nil

	var deadLetterCount int
	if err := s.pool.QueryRow(ctx, `
		select count(*)
		from dead_letters
		where workspace_id = $1
		  and created_at >= now() - interval '7 days'
	`, workspaceID).Scan(&deadLetterCount); err != nil {
		return WorkspaceOpsSummary{}, err
	}
	summary.DeadLetterCount = deadLetterCount

	rows, err := s.pool.Query(ctx, workspaceOpsSummaryOutboxQuery, workspaceID)
	if err != nil {
		return WorkspaceOpsSummary{}, err
	}
	defer rows.Close()
	var oldestPending *int
	now := time.Now().UTC()
	for rows.Next() {
		var kind string
		var status string
		var availableAt time.Time
		var failedAt *time.Time
		if err := rows.Scan(&kind, &status, &availableAt, &failedAt); err != nil {
			return WorkspaceOpsSummary{}, err
		}
		if status == "retrying" || status == "pending" || status == "processing" {
			summary.RetryBacklogCount++
			age := int(now.Sub(availableAt).Minutes())
			if age < 0 {
				age = 0
			}
			if oldestPending == nil || age > *oldestPending {
				oldestPending = &age
			}
		}
		switch kind {
		case "triage_card_refresh":
			if failedAt != nil {
				summary.FailedSlackRefreshes++
			}
		case "escalation_notification":
			if failedAt != nil {
				summary.FailedEscalations++
			}
		case "external_sync_request", "tracker_link_request":
			if failedAt != nil {
				summary.FailedTrackerSyncs++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return WorkspaceOpsSummary{}, err
	}
	summary.OldestPendingAgeMins = oldestPending

	var syncLag *int
	if err := s.pool.QueryRow(ctx, workspaceOpsSyncLagQuery, workspaceID).Scan(&syncLag); err != nil && err != pgx.ErrNoRows {
		return WorkspaceOpsSummary{}, err
	}
	summary.SyncLagMinutes = syncLag

	summary.HealthBanner = workspaceOpsHealthBanner(summary)
	return summary, nil
}

func (s *Store) InternalOpsSummary(ctx context.Context) (InternalOpsSummary, error) {
	var summary InternalOpsSummary
	runners, err := s.ListJobRuntimeStatuses(ctx)
	if err != nil {
		return InternalOpsSummary{}, err
	}
	summary.Runners = runners
	if err := s.pool.QueryRow(ctx, `
		select count(*) from dead_letters where created_at >= now() - interval '7 days'
	`).Scan(&summary.DeadLetterCount); err != nil {
		return InternalOpsSummary{}, err
	}
	if err := s.pool.QueryRow(ctx, internalOpsSummaryOutboxQuery).Scan(&summary.FailedOutboxCount, &summary.RetryBacklogCount, &summary.OldestPendingAgeMins, &summary.WorkspacesAffected); err != nil {
		return InternalOpsSummary{}, err
	}
	return summary, nil
}

func workspaceOpsHealthBanner(summary WorkspaceOpsSummary) string {
	switch {
	case !summary.SlackConnected:
		return "Slack is not connected. Queue control is disabled until Slack is reconnected."
	case summary.DeadLetterCount > 0 || summary.FailedTrackerSyncs > 0 || summary.FailedEscalations > 0:
		return fmt.Sprintf("Attention needed: %d dead letters, %d failed escalations, %d failed tracker syncs in the last 7 days.", summary.DeadLetterCount, summary.FailedEscalations, summary.FailedTrackerSyncs)
	case summary.RetryBacklogCount > 0:
		return fmt.Sprintf("Processing backlog: %d pending or retrying side effects.", summary.RetryBacklogCount)
	default:
		return "All integrations and queue-control side effects are healthy."
	}
}

func (s *Store) ListRecentDailyQueueMetrics(ctx context.Context, queueID uuid.UUID, days int) ([]DailyQueueMetric, error) {
	if days <= 0 {
		days = 14
	}
	if days > 90 {
		days = 90
	}
	rows, err := s.pool.Query(ctx, `
		select date::text, workspace_id, queue_id, created_count, closed_count, unacked_count, unassigned_count,
			breached_ack_count, breached_assign_count, median_ack_minutes, median_assign_minutes, median_close_minutes, reopen_count
		from daily_queue_metrics
		where queue_id = $1
		  and date >= current_date - ($2::int - 1)
		order by date asc
	`, queueID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]DailyQueueMetric, 0, days)
	for rows.Next() {
		var item DailyQueueMetric
		if err := rows.Scan(
			&item.Date,
			&item.WorkspaceID,
			&item.QueueID,
			&item.CreatedCount,
			&item.ClosedCount,
			&item.UnackedCount,
			&item.UnassignedCount,
			&item.BreachedAckCount,
			&item.BreachedAssignCount,
			&item.MedianAckMinutes,
			&item.MedianAssignMinutes,
			&item.MedianCloseMinutes,
			&item.ReopenCount,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) GetDemoSeedRun(ctx context.Context, workspaceID uuid.UUID) (DemoSeedRun, error) {
	var item DemoSeedRun
	err := s.pool.QueryRow(ctx, `
		select id, workspace_id, manifest_json, created_by, created_at, updated_at
		from demo_seed_runs
		where workspace_id = $1
	`, workspaceID).Scan(&item.ID, &item.WorkspaceID, &item.Manifest, &item.CreatedBy, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return DemoSeedRun{}, err
	}
	return item, nil
}

func (s *Store) DeleteDemoSeedRun(ctx context.Context, workspaceID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `delete from demo_seed_runs where workspace_id = $1`, workspaceID)
	return err
}

func (s *Store) ResetDemoWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	run, err := s.GetDemoSeedRun(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	}
	var manifest struct {
		RequestIDs []string `json:"request_ids"`
		QueueIDs   []string `json:"queue_ids"`
	}
	if err := json.Unmarshal(run.Manifest, &manifest); err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, id := range manifest.RequestIDs {
		requestID, parseErr := uuid.Parse(strings.TrimSpace(id))
		if parseErr != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `delete from requests where workspace_id = $1 and id = $2`, workspaceID, requestID); err != nil {
			return err
		}
	}
	for _, id := range manifest.QueueIDs {
		queueID, parseErr := uuid.Parse(strings.TrimSpace(id))
		if parseErr != nil {
			continue
		}
		if _, err := tx.Exec(ctx, `delete from queues where workspace_id = $1 and id = $2`, workspaceID, queueID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `delete from demo_seed_runs where workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SeedDemoWorkspace(ctx context.Context, workspaceID uuid.UUID, createdBy *uuid.UUID, sharedPolicy *Policy) error {
	if err := s.ResetDemoWorkspace(ctx, workspaceID); err != nil {
		return err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	type demoQueue struct {
		id   uuid.UUID
		name string
	}
	queueSpecs := []demoQueue{
		{id: uuid.New(), name: "Platform requests"},
		{id: uuid.New(), name: "Security help"},
	}
	queueIDs := make([]string, 0, len(queueSpecs))
	for idx, spec := range queueSpecs {
		isDefault := idx == 0
		if _, err := tx.Exec(ctx, `
			insert into queues (
				id, workspace_id, name, description, default_priority, default_request_type, escalation_channel_id, is_default, created_at, updated_at
			) values ($1, $2, $3, $4, 'P2', 'other', null, $5, now(), now())
		`, spec.id, workspaceID, spec.name, fmt.Sprintf("Sample queue for %s", spec.name), isDefault); err != nil {
			return err
		}
		queueIDs = append(queueIDs, spec.id.String())
		if sharedPolicy != nil {
			if _, err := tx.Exec(ctx, `
				insert into queue_policies (
					queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours, digest_time, digest_weekday,
					assign_starts_from, stale_starts_from, timezone, business_hours_enabled, business_hours_start,
					business_hours_end, business_days_mask, created_at, updated_at
				) values ($1, $2, $3, $4, $5, null, 'created_at', 'last_human_activity_at', $6, false, null, null, 62, now(), now())
				on conflict (queue_id) do update set
					ack_sla_minutes = excluded.ack_sla_minutes,
					assign_sla_minutes = excluded.assign_sla_minutes,
					stale_hours = excluded.stale_hours,
					digest_time = excluded.digest_time,
					timezone = excluded.timezone,
					updated_at = now()
			`, spec.id, sharedPolicy.AckSLAMinutes, sharedPolicy.AssignSLAMinutes, sharedPolicy.StaleHours, sharedPolicy.DailyDigestTime, sharedPolicy.Timezone); err != nil {
				return err
			}
		} else {
			if _, err := tx.Exec(ctx, `
				insert into queue_policies (
					queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours, digest_time, digest_weekday,
					assign_starts_from, stale_starts_from, timezone, business_hours_enabled, business_hours_start,
					business_hours_end, business_days_mask, created_at, updated_at
				) values (
					$1, $2, $3, $4, '09:00:00', $5, 'created_at', 'last_human_activity_at', 'Europe/Madrid',
					$6, $7::time, $8::time, 62, now(), now()
				)
				on conflict (queue_id) do update set
					ack_sla_minutes = excluded.ack_sla_minutes,
					assign_sla_minutes = excluded.assign_sla_minutes,
					stale_hours = excluded.stale_hours,
					digest_time = excluded.digest_time,
					digest_weekday = excluded.digest_weekday,
					timezone = excluded.timezone,
					business_hours_enabled = excluded.business_hours_enabled,
					business_hours_start = excluded.business_hours_start,
					business_hours_end = excluded.business_hours_end,
					updated_at = now()
			`, spec.id, 15+idx*15, 45+idx*30, 24+idx*12, intPtr(1+idx), idx == 0, strPtr("09:00:00"), strPtr("18:00:00")); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `
			insert into queue_members (queue_id, user_id, role, created_at, updated_at)
			values ($1, 'UDEMO_TRIAGER', 'triager', now(), now()),
			       ($1, 'UDEMO_MANAGER', 'manager', now(), now())
			on conflict do nothing
		`, spec.id); err != nil {
			return err
		}
	}

	type demoRequest struct {
		id               uuid.UUID
		queueID          uuid.UUID
		sourceKey        string
		channelID        string
		threadTS         string
		messageTS        string
		authorSlackID    string
		bodyText         string
		title            string
		status           string
		priority         string
		ownerUserID      *string
		dueAt            *time.Time
		ackedAt          *time.Time
		assignedAt       *time.Time
		resolvedAt       *time.Time
		state            string
		requestType      string
		waitingOn        string
		snoozedUntil     *time.Time
		closedAt         *time.Time
		closedReason     *string
		lastHuman        time.Time
		createdAt        time.Time
		ackClockState    string
		ackTarget        *time.Time
		assignClockState string
		assignTarget     *time.Time
		staleClockState  string
		staleTarget      *time.Time
	}
	ownerA := strPtr("UDEMO_OWNER_A")
	ownerB := strPtr("UDEMO_OWNER_B")
	resolvedReason := strPtr(ClosedReasonResolved)
	r1Created := now.Add(-40 * time.Minute)
	r2Created := now.Add(-26 * time.Hour)
	r2Ack := now.Add(-25 * time.Hour)
	r2Assign := now.Add(-24 * time.Hour)
	r2Snooze := now.Add(18 * time.Hour)
	r3Created := now.Add(-3 * 24 * time.Hour)
	r3Ack := now.Add(-3*24*time.Hour + 10*time.Minute)
	r3Assign := now.Add(-3*24*time.Hour + 30*time.Minute)
	r3Closed := now.Add(-2 * 24 * time.Hour)
	r4Created := now.Add(-52 * time.Hour)
	r4Ack := now.Add(-51*time.Hour + 30*time.Minute)
	r4Assign := now.Add(-50 * time.Hour)
	r5Created := now.Add(-5 * time.Hour)
	r5Ack := now.Add(-4*time.Hour + 15*time.Minute)
	r6Created := now.Add(-36 * time.Hour)
	r6Ack := now.Add(-35*time.Hour + 15*time.Minute)
	r6Assign := now.Add(-35 * time.Hour)
	requests := []demoRequest{
		{
			id:               uuid.New(),
			queueID:          queueSpecs[0].id,
			sourceKey:        fmt.Sprintf("demo:%s:1", workspaceID),
			channelID:        "DEMO-PLATFORM",
			threadTS:         slackTS(r1Created),
			messageTS:        slackTS(r1Created),
			authorSlackID:    "UDEMO_REQ1",
			bodyText:         "API access request is still waiting for ack and owner.",
			title:            "Billing API access blocked",
			status:           "NEW",
			priority:         "P1",
			state:            RequestStateOpen,
			requestType:      "access",
			waitingOn:        WaitingOnNone,
			lastHuman:        r1Created,
			createdAt:        r1Created,
			ackClockState:    "breached",
			ackTarget:        timePtr(r1Created.Add(15 * time.Minute)),
			assignClockState: "stopped",
			staleClockState:  "stopped",
		},
		{
			id:               uuid.New(),
			queueID:          queueSpecs[0].id,
			sourceKey:        fmt.Sprintf("demo:%s:2", workspaceID),
			channelID:        "DEMO-PLATFORM",
			threadTS:         slackTS(r2Created),
			messageTS:        slackTS(r2Created),
			authorSlackID:    "UDEMO_REQ2",
			bodyText:         "Waiting on requester for screenshots before continuing.",
			title:            "Grafana dashboard renders empty",
			status:           "ASSIGNED",
			priority:         "P2",
			ownerUserID:      ownerA,
			ackedAt:          &r2Ack,
			assignedAt:       &r2Assign,
			state:            RequestStateOpen,
			requestType:      "bug",
			waitingOn:        WaitingOnRequester,
			snoozedUntil:     &r2Snooze,
			lastHuman:        r2Assign,
			createdAt:        r2Created,
			ackClockState:    "satisfied",
			assignClockState: "satisfied",
			staleClockState:  "paused",
		},
		{
			id:               uuid.New(),
			queueID:          queueSpecs[0].id,
			sourceKey:        fmt.Sprintf("demo:%s:3", workspaceID),
			channelID:        "DEMO-PLATFORM",
			threadTS:         slackTS(r3Created),
			messageTS:        slackTS(r3Created),
			authorSlackID:    "UDEMO_REQ3",
			bodyText:         "Resolved by rotating the credential.",
			title:            "Deploy key expired",
			status:           "RESOLVED",
			priority:         "P1",
			ownerUserID:      ownerA,
			ackedAt:          &r3Ack,
			assignedAt:       &r3Assign,
			resolvedAt:       &r3Closed,
			state:            RequestStateClosed,
			requestType:      "infra",
			waitingOn:        WaitingOnNone,
			closedAt:         &r3Closed,
			closedReason:     resolvedReason,
			lastHuman:        r3Closed,
			createdAt:        r3Created,
			ackClockState:    "satisfied",
			assignClockState: "satisfied",
			staleClockState:  "stopped",
		},
		{
			id:               uuid.New(),
			queueID:          queueSpecs[1].id,
			sourceKey:        fmt.Sprintf("demo:%s:4", workspaceID),
			channelID:        "DEMO-SECURITY",
			threadTS:         slackTS(r4Created),
			messageTS:        slackTS(r4Created),
			authorSlackID:    "UDEMO_REQ4",
			bodyText:         "Suspicious login needs triage follow-up.",
			title:            "Multiple failed SSO attempts",
			status:           "ASSIGNED",
			priority:         "P0",
			ownerUserID:      ownerB,
			ackedAt:          &r4Ack,
			assignedAt:       &r4Assign,
			state:            RequestStateOpen,
			requestType:      "incident",
			waitingOn:        WaitingOnNone,
			lastHuman:        now.Add(-48 * time.Hour),
			createdAt:        r4Created,
			ackClockState:    "satisfied",
			assignClockState: "satisfied",
			staleClockState:  "breached",
			staleTarget:      timePtr(now.Add(-20 * time.Hour)),
		},
		{
			id:               uuid.New(),
			queueID:          queueSpecs[1].id,
			sourceKey:        fmt.Sprintf("demo:%s:5", workspaceID),
			channelID:        "DEMO-SECURITY",
			threadTS:         slackTS(r5Created),
			messageTS:        slackTS(r5Created),
			authorSlackID:    "UDEMO_REQ5",
			bodyText:         "Security questionnaire answered, owner still missing.",
			title:            "Vendor due diligence request",
			status:           "ACKED",
			priority:         "P2",
			ackedAt:          &r5Ack,
			state:            RequestStateOpen,
			requestType:      "question",
			waitingOn:        WaitingOnNone,
			lastHuman:        r5Ack,
			createdAt:        r5Created,
			ackClockState:    "satisfied",
			assignClockState: "running",
			assignTarget:     timePtr(r5Ack.Add(30 * time.Minute)),
			staleClockState:  "running",
			staleTarget:      timePtr(now.Add(6 * time.Hour)),
		},
		{
			id:               uuid.New(),
			queueID:          queueSpecs[1].id,
			sourceKey:        fmt.Sprintf("demo:%s:6", workspaceID),
			channelID:        "DEMO-SECURITY",
			threadTS:         slackTS(r6Created),
			messageTS:        slackTS(r6Created),
			authorSlackID:    "UDEMO_REQ6",
			bodyText:         "Duplicate of earlier access review thread, later reopened for more evidence.",
			title:            "Reopened access review",
			status:           "ASSIGNED",
			priority:         "P1",
			ownerUserID:      ownerB,
			ackedAt:          &r6Ack,
			assignedAt:       &r6Assign,
			state:            RequestStateOpen,
			requestType:      "access",
			waitingOn:        WaitingOnNone,
			lastHuman:        now.Add(-20 * time.Hour),
			createdAt:        r6Created,
			ackClockState:    "satisfied",
			assignClockState: "satisfied",
			staleClockState:  "running",
			staleTarget:      timePtr(now.Add(3 * time.Hour)),
		},
	}
	requestIDs := make([]string, 0, len(requests))
	for _, item := range requests {
		title := item.title
		if _, err := tx.Exec(ctx, `
			insert into requests (
				id, workspace_id, source_key, channel_id, thread_ts, message_ts, author_slack_id, body_text, title,
				status, priority, owner_slack_id, due_at, acked_at, assigned_at, resolved_at, last_activity_at,
				ack_overdue_sent_at, assign_overdue_sent_at, stale_sent_at, triage_message_ts, created_at, state,
				acknowledged_at, acknowledged_by, owner_user_id, request_type, queue_id, waiting_on, snoozed_until,
				closed_at, closed_by, closed_reason, last_human_activity_at, last_state_change_at
			) values (
				$1, $2, $3, $4, $5, $6, $7, $8, $9,
				$10, $11, $12, $13, $14, $15, $16, $17,
				null, null, null, null, $18, $19,
				$20, 'UDEMO_TRIAGER', $21, $22, $23, $24, $25,
				$26, 'UDEMO_TRIAGER', $27, $28, $29
			)
		`, item.id, workspaceID, item.sourceKey, item.channelID, item.threadTS, item.messageTS, item.authorSlackID, item.bodyText, title,
			item.status, item.priority, item.ownerUserID, item.dueAt, item.ackedAt, item.assignedAt, item.resolvedAt, item.lastHuman,
			item.createdAt, item.state, item.ackedAt, item.ownerUserID, item.requestType, item.queueID, item.waitingOn, item.snoozedUntil,
			item.closedAt, item.closedReason, item.lastHuman, item.lastHuman); err != nil {
			return err
		}
		requestIDs = append(requestIDs, item.id.String())
		if _, err := tx.Exec(ctx, `
			insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
			values ($1, 'request_created', 'system', null, $2, '{}'::jsonb)
		`, item.id, item.createdAt); err != nil {
			return err
		}
		if item.ackedAt != nil {
			if _, err := tx.Exec(ctx, `
				insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
				values ($1, 'request_acknowledged', 'slack_user', 'UDEMO_TRIAGER', $2, '{}'::jsonb)
			`, item.id, *item.ackedAt); err != nil {
				return err
			}
		}
		if item.assignedAt != nil && item.ownerUserID != nil {
			if _, err := tx.Exec(ctx, `
				insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
				values ($1, 'owner_assigned', 'slack_user', 'UDEMO_TRIAGER', $2, jsonb_build_object('owner_user_id', $3))
			`, item.id, *item.assignedAt, *item.ownerUserID); err != nil {
				return err
			}
		}
		if item.waitingOn != WaitingOnNone {
			if _, err := tx.Exec(ctx, `
				insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
				values ($1, 'waiting_set', 'slack_user', 'UDEMO_OWNER_A', $2, jsonb_build_object('waiting_on', $3, 'comment', 'Sample waiting state'))
			`, item.id, item.lastHuman, item.waitingOn); err != nil {
				return err
			}
		}
		if item.id == requests[5].id {
			if _, err := tx.Exec(ctx, `
				insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
				values ($1, 'request_reopened', 'slack_user', 'UDEMO_MANAGER', $2, '{}'::jsonb)
			`, item.id, now.Add(-18*time.Hour)); err != nil {
				return err
			}
		}
		if item.state == RequestStateClosed && item.closedReason != nil {
			if _, err := tx.Exec(ctx, `
				insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
				values ($1, 'request_closed', 'slack_user', 'UDEMO_TRIAGER', $2, jsonb_build_object('closed_reason', $3))
			`, item.id, *item.closedAt, *item.closedReason); err != nil {
				return err
			}
		}
		clockRows := []struct {
			clockType string
			state     string
			startedAt time.Time
			targetAt  *time.Time
		}{
			{clockType: "ack", state: item.ackClockState, startedAt: item.createdAt, targetAt: item.ackTarget},
			{clockType: "assign", state: item.assignClockState, startedAt: coalesceTimePtr(item.ackedAt, item.createdAt), targetAt: item.assignTarget},
			{clockType: "stale", state: item.staleClockState, startedAt: item.lastHuman, targetAt: item.staleTarget},
		}
		for _, clock := range clockRows {
			if clock.state == "" {
				continue
			}
			var pausedAt *time.Time
			var satisfiedAt *time.Time
			var breachedAt *time.Time
			switch clock.state {
			case "paused":
				pausedAt = item.snoozedUntil
			case "satisfied":
				satisfiedAt = timePtr(coalesceTimePtr(item.assignedAt, coalesceTimePtr(item.ackedAt, item.createdAt)))
			case "breached":
				breachedAt = clock.targetAt
			case "stopped":
				satisfiedAt = item.closedAt
			}
			if _, err := tx.Exec(ctx, `
				insert into request_sla_clocks (
					request_id, clock_type, started_at, target_at, paused_at, satisfied_at, breached_at, last_notified_at, state, created_at
				) values ($1, $2, $3, $4, $5, $6, $7, null, $8, now())
			`, item.id, clock.clockType, clock.startedAt, clock.targetAt, pausedAt, satisfiedAt, breachedAt, clock.state); err != nil {
				return err
			}
		}
	}

	if _, err := tx.Exec(ctx, `
		insert into external_issues (
			request_id, provider, external_id, external_key, external_url, sync_state, last_sync_at, last_sync_error, created_at, updated_at
		) values
		($1, 'linear', 'demo-linear-1', 'SEC-42', 'https://linear.app/demo/issue/SEC-42', 'error', $2, 'Sample sync lag for demo', now(), now())
	`, requests[3].id, now.Add(-190*time.Minute)); err != nil {
		return err
	}

	manifest, _ := json.Marshal(map[string]any{
		"queue_ids":   queueIDs,
		"request_ids": requestIDs,
	})
	if _, err := tx.Exec(ctx, `
		insert into demo_seed_runs (workspace_id, manifest_json, created_by, created_at, updated_at)
		values ($1, $2::jsonb, $3, now(), now())
		on conflict (workspace_id) do update set
			manifest_json = excluded.manifest_json,
			created_by = excluded.created_by,
			updated_at = now()
	`, workspaceID, manifest, createdBy); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func intPtr(value int) *int {
	return &value
}

func strPtr(value string) *string {
	return &value
}

func timePtr(value time.Time) *time.Time {
	return &value
}

func coalesceTimePtr(primary *time.Time, fallback ...time.Time) time.Time {
	if primary != nil && !primary.IsZero() {
		return *primary
	}
	for _, value := range fallback {
		if !value.IsZero() {
			return value
		}
	}
	return time.Now().UTC()
}

func slackTS(t time.Time) string {
	utc := t.UTC()
	return fmt.Sprintf("%d.%06d", utc.Unix(), utc.Nanosecond()/1000)
}
