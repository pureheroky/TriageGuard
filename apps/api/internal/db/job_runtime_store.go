package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListAllQueues(ctx context.Context) ([]Queue, error) {
	rows, err := s.pool.Query(ctx, `
		select id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
		from queues
		order by workspace_id asc, is_default desc, name asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Queue, 0)
	for rows.Next() {
		var item Queue
		if err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&item.Name,
			&item.Description,
			&item.DefaultPriority,
			&item.DefaultRequestType,
			&item.EscalationChannelID,
			&item.IsDefault,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListOpenRequestsForClockEvaluation(ctx context.Context) ([]Request, error) {
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		select %s
		from requests
		where state = 'OPEN'
		order by created_at asc
	`, requestSelectColumns("")))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Request, 0)
	for rows.Next() {
		var item Request
		if err := scanRequest(rows, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ReconcileRequestClocks(ctx context.Context, requestID uuid.UUID, now time.Time) (Request, []RequestSLAClock, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Request{}, nil, err
	}
	defer tx.Rollback(ctx)

	req, err := getRequestByIDTx(ctx, tx, requestID)
	if err != nil {
		return Request{}, nil, err
	}
	policy, err := getQueuePolicyForRequestTx(ctx, tx, req)
	if err != nil {
		return Request{}, nil, err
	}
	if err := syncClockPhaseTx(ctx, tx, req, policy, now, false); err != nil {
		return Request{}, nil, err
	}

	rows, err := tx.Query(ctx, `
		select distinct on (clock_type)
			id, request_id, clock_type, started_at, target_at, paused_at,
			satisfied_at, breached_at, last_notified_at, state, created_at
		from request_sla_clocks
		where request_id = $1
		order by clock_type asc, created_at desc
	`, requestID)
	if err != nil {
		return Request{}, nil, err
	}
	defer rows.Close()

	clocks := make([]RequestSLAClock, 0, 3)
	for rows.Next() {
		var item RequestSLAClock
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.ClockType,
			&item.StartedAt,
			&item.TargetAt,
			&item.PausedAt,
			&item.SatisfiedAt,
			&item.BreachedAt,
			&item.LastNotifiedAt,
			&item.State,
			&item.CreatedAt,
		); err != nil {
			return Request{}, nil, err
		}
		clocks = append(clocks, item)
	}
	if err := rows.Err(); err != nil {
		return Request{}, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Request{}, nil, err
	}
	ApplyRequestCompatibility(&req)
	return req, clocks, nil
}

func (s *Store) MarkClockBreached(ctx context.Context, clockID uuid.UUID, breachedAt time.Time) (RequestSLAClock, error) {
	var item RequestSLAClock
	err := s.pool.QueryRow(ctx, `
		update request_sla_clocks
		set breached_at = coalesce(breached_at, $2),
			state = 'breached'
		where id = $1
		returning id, request_id, clock_type, started_at, target_at, paused_at,
			satisfied_at, breached_at, last_notified_at, state, created_at
	`, clockID, breachedAt).Scan(
		&item.ID,
		&item.RequestID,
		&item.ClockType,
		&item.StartedAt,
		&item.TargetAt,
		&item.PausedAt,
		&item.SatisfiedAt,
		&item.BreachedAt,
		&item.LastNotifiedAt,
		&item.State,
		&item.CreatedAt,
	)
	if err != nil {
		return RequestSLAClock{}, err
	}
	return item, nil
}

func (s *Store) SetClockLastNotified(ctx context.Context, clockID uuid.UUID, notifiedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		update request_sla_clocks
		set last_notified_at = $2
		where id = $1
	`, clockID, notifiedAt)
	return err
}

func (s *Store) ListEscalationSteps(ctx context.Context, queueID uuid.UUID, clockType string) ([]EscalationStep, error) {
	rows, err := s.pool.Query(ctx, `
		select id, queue_id, clock_type, delay_minutes_after_breach, target_type, target_value,
			message_template, is_enabled, created_at, updated_at
		from escalation_steps
		where queue_id = $1 and clock_type = $2 and is_enabled = true
		order by delay_minutes_after_breach asc, created_at asc
	`, queueID, strings.TrimSpace(clockType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]EscalationStep, 0)
	for rows.Next() {
		var item EscalationStep
		if err := rows.Scan(
			&item.ID,
			&item.QueueID,
			&item.ClockType,
			&item.DelayMinutesAfterBreach,
			&item.TargetType,
			&item.TargetValue,
			&item.MessageTemplate,
			&item.IsEnabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ClaimNotificationOutbox(ctx context.Context, limit int) ([]NotificationOutboxItem, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 200 {
		limit = 200
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		with picked as (
			select id
			from notification_outbox
			where status in ('pending', 'retrying')
			  and available_at <= now()
			order by available_at asc, created_at asc
			for update skip locked
			limit $1
		)
		update notification_outbox o
		set status = 'processing',
			updated_at = now()
		from picked
		where o.id = picked.id
		returning o.id, o.request_id, o.event_id, o.kind, o.destination_type, o.destination_value,
			o.payload_json, o.dedupe_key, o.status, o.retry_count, o.available_at, o.sent_at,
			o.failed_at, o.last_error, o.created_at, o.updated_at
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) MarkNotificationOutboxSent(ctx context.Context, itemID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		update notification_outbox
		set status = 'sent',
			sent_at = now(),
			failed_at = null,
			last_error = null,
			updated_at = now()
		where id = $1
	`, itemID)
	return err
}

func (s *Store) MarkNotificationOutboxRetry(ctx context.Context, itemID uuid.UUID, lastError string, availableAt time.Time, terminal bool) error {
	status := "retrying"
	if terminal {
		status = "failed"
	}
	_, err := s.pool.Exec(ctx, `
		update notification_outbox
		set status = $2,
			retry_count = retry_count + 1,
			available_at = $3,
			failed_at = case when $4 then now() else failed_at end,
			last_error = $5,
			updated_at = now()
		where id = $1
	`, itemID, status, availableAt, terminal, strings.TrimSpace(lastError))
	return err
}

func (s *Store) ListQueueOpenRequests(ctx context.Context, queueID uuid.UUID, limit int) ([]Request, error) {
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		select %s
		from requests
		where queue_id = $1
		  and state = 'OPEN'
		order by created_at asc
		limit $2
	`, requestSelectColumns("")), queueID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Request, 0, limit)
	for rows.Next() {
		var item Request
		if err := scanRequest(rows, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertDailyQueueMetric(ctx context.Context, metric DailyQueueMetric) error {
	_, err := s.pool.Exec(ctx, `
		insert into daily_queue_metrics (
			date, workspace_id, queue_id, created_count, closed_count, unacked_count, unassigned_count,
			breached_ack_count, breached_assign_count, median_ack_minutes, median_assign_minutes,
			median_close_minutes, reopen_count, created_at, updated_at
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now()
		)
		on conflict (date, workspace_id, queue_id) do update set
			created_count = excluded.created_count,
			closed_count = excluded.closed_count,
			unacked_count = excluded.unacked_count,
			unassigned_count = excluded.unassigned_count,
			breached_ack_count = excluded.breached_ack_count,
			breached_assign_count = excluded.breached_assign_count,
			median_ack_minutes = excluded.median_ack_minutes,
			median_assign_minutes = excluded.median_assign_minutes,
			median_close_minutes = excluded.median_close_minutes,
			reopen_count = excluded.reopen_count,
			updated_at = now()
	`, metric.Date, metric.WorkspaceID, metric.QueueID, metric.CreatedCount, metric.ClosedCount, metric.UnackedCount, metric.UnassignedCount, metric.BreachedAckCount, metric.BreachedAssignCount, metric.MedianAckMinutes, metric.MedianAssignMinutes, metric.MedianCloseMinutes, metric.ReopenCount)
	return err
}

func (s *Store) ComputeDailyQueueMetric(ctx context.Context, queue Queue, day time.Time) (DailyQueueMetric, error) {
	start := time.Date(day.UTC().Year(), day.UTC().Month(), day.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)

	var metric DailyQueueMetric
	metric.Date = start.Format("2006-01-02")
	metric.WorkspaceID = queue.WorkspaceID
	metric.QueueID = queue.ID

	err := s.pool.QueryRow(ctx, `
		with scope as (
			select *
			from requests
			where queue_id = $1
		),
		reopen_counts as (
			select count(*)::int as reopen_count
			from request_events
			where event_type = 'request_reopened'
			  and occurred_at >= $2
			  and occurred_at < $3
			  and request_id in (select id from scope)
		)
		select
			count(*) filter (where created_at >= $2 and created_at < $3)::int as created_count,
			count(*) filter (where closed_at >= $2 and closed_at < $3)::int as closed_count,
			count(*) filter (
				where state = 'OPEN'
				  and created_at < $3
				  and (acknowledged_at is null or acknowledged_at >= $3)
			)::int as unacked_count,
			count(*) filter (
				where state = 'OPEN'
				  and created_at < $3
				  and (owner_user_id is null or btrim(owner_user_id) = '')
			)::int as unassigned_count,
			count(*) filter (
				where exists (
					select 1
					from request_sla_clocks c
					where c.request_id = scope.id
					  and c.clock_type = 'ack'
					  and c.breached_at is not null
					  and c.breached_at < $3
				)
			)::int as breached_ack_count,
			count(*) filter (
				where exists (
					select 1
					from request_sla_clocks c
					where c.request_id = scope.id
					  and c.clock_type = 'assign'
					  and c.breached_at is not null
					  and c.breached_at < $3
				)
			)::int as breached_assign_count,
			percentile_cont(0.5) within group (
				order by extract(epoch from (acknowledged_at - created_at)) / 60.0
			) filter (
				where acknowledged_at is not null
				  and acknowledged_at >= $2
				  and acknowledged_at < $3
			) as median_ack_minutes,
			percentile_cont(0.5) within group (
				order by extract(epoch from (assigned_at - coalesce(acknowledged_at, created_at))) / 60.0
			) filter (
				where assigned_at is not null
				  and assigned_at >= $2
				  and assigned_at < $3
			) as median_assign_minutes,
			percentile_cont(0.5) within group (
				order by extract(epoch from (closed_at - created_at)) / 60.0
			) filter (
				where closed_at is not null
				  and closed_at >= $2
				  and closed_at < $3
			) as median_close_minutes,
			(select reopen_count from reopen_counts)
		from scope
	`, queue.ID, start, end).Scan(
		&metric.CreatedCount,
		&metric.ClosedCount,
		&metric.UnackedCount,
		&metric.UnassignedCount,
		&metric.BreachedAckCount,
		&metric.BreachedAssignCount,
		&metric.MedianAckMinutes,
		&metric.MedianAssignMinutes,
		&metric.MedianCloseMinutes,
		&metric.ReopenCount,
	)
	if err != nil {
		return DailyQueueMetric{}, err
	}
	return metric, nil
}
