package db

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ApplyRequestMutation(ctx context.Context, mutation RequestMutation) (Request, []RequestEvent, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Request{}, nil, err
	}
	defer tx.Rollback(ctx)

	req, err := getRequestByIDTx(ctx, tx, mutation.RequestID)
	if err != nil {
		return Request{}, nil, err
	}

	now := time.Now().UTC()
	events := make([]RequestEvent, 0, 8)
	humanActor := strings.TrimSpace(mutation.ActorType) == "slack_user"
	touchHuman := false
	queueChanged := false
	trackerSyncNeeded := false
	linkTrackerRequested := mutation.LinkTracker != nil && *mutation.LinkTracker

	addEvent := func(eventType string, payload map[string]any) {
		if payload == nil {
			payload = map[string]any{}
		}
		data, _ := json.Marshal(payload)
		events = append(events, RequestEvent{
			RequestID:  req.ID,
			EventType:  eventType,
			ActorType:  fallbackActorType(mutation.ActorType),
			ActorID:    mutation.ActorID,
			OccurredAt: now,
			Payload:    data,
		})
	}

	if mutation.Reopen && NormalizeRequestState(req.State) == RequestStateClosed {
		req.State = RequestStateOpen
		req.ClosedAt = nil
		req.ClosedBy = nil
		req.ClosedReason = nil
		req.SnoozedUntil = nil
		req.WaitingOn = WaitingOnNone
		if req.AcknowledgedAt == nil {
			req.AcknowledgedAt = &now
			req.AcknowledgedBy = mutation.ActorID
		}
		req.LastStateChangeAt = now
		addEvent("request_reopened", nil)
		trackerSyncNeeded = true
	}

	if mutation.CloseReason != nil {
		reason := normalizeCloseReason(*mutation.CloseReason)
		req.State = RequestStateClosed
		req.ClosedAt = &now
		req.ClosedBy = mutation.ActorID
		req.ClosedReason = &reason
		req.WaitingOn = WaitingOnNone
		req.SnoozedUntil = nil
		req.LastStateChangeAt = now
		addEvent("request_closed", map[string]any{"closed_reason": reason})
		trackerSyncNeeded = true
	}

	if mutation.Acknowledge != nil && *mutation.Acknowledge && req.AcknowledgedAt == nil {
		req.AcknowledgedAt = &now
		req.AcknowledgedBy = mutation.ActorID
		req.LastStateChangeAt = now
		addEvent("request_acknowledged", nil)
		touchHuman = true
		trackerSyncNeeded = true
	}

	if mutation.OwnerUserID != nil {
		normalized := normalizeOptionalString(mutation.OwnerUserID)
		if !sameOptionalString(req.OwnerUserID, normalized) {
			req.OwnerUserID = normalized
			req.OwnerSlackID = normalized
			req.LastStateChangeAt = now
			if normalized != nil {
				req.AssignedAt = &now
				addEvent("owner_assigned", map[string]any{"owner_user_id": *normalized})
			} else {
				req.AssignedAt = nil
				addEvent("owner_cleared", nil)
			}
			touchHuman = true
			trackerSyncNeeded = true
		}
	}

	if mutation.Priority != nil {
		priority := normalizedPriority(*mutation.Priority)
		if req.Priority != priority {
			req.Priority = priority
			req.LastStateChangeAt = now
			addEvent("priority_changed", map[string]any{"priority": priority})
			touchHuman = true
			trackerSyncNeeded = true
		}
	}

	if mutation.RequestType != nil {
		requestType := normalizeRequestType(*mutation.RequestType)
		if req.RequestType != requestType {
			req.RequestType = requestType
			req.LastStateChangeAt = now
			addEvent("type_changed", map[string]any{"request_type": requestType})
			touchHuman = true
		}
	}

	if mutation.QueueID != nil && (req.QueueID == nil || *req.QueueID != *mutation.QueueID) {
		req.QueueID = mutation.QueueID
		req.LastStateChangeAt = now
		queueChanged = true
		addEvent("queue_changed", map[string]any{"queue_id": mutation.QueueID.String()})
		touchHuman = true
	}

	if mutation.DueAt != nil {
		nextDue := *mutation.DueAt
		if !sameTimePtr(req.DueAt, nextDue) {
			req.DueAt = nextDue
			req.LastStateChangeAt = now
			addEvent("due_changed", map[string]any{"due_at": nextDue})
			touchHuman = true
			trackerSyncNeeded = true
		}
	}

	if mutation.WaitingOn != nil {
		waitingOn := NormalizeWaitingOn(*mutation.WaitingOn)
		if req.WaitingOn != waitingOn {
			req.WaitingOn = waitingOn
			req.LastStateChangeAt = now
			payload := map[string]any{"waiting_on": waitingOn}
			if mutation.Comment != nil && strings.TrimSpace(*mutation.Comment) != "" {
				payload["comment"] = strings.TrimSpace(*mutation.Comment)
			}
			addEvent("waiting_set", payload)
			touchHuman = true
		}
	}

	if mutation.SnoozedUntil != nil {
		nextSnooze := *mutation.SnoozedUntil
		if !sameTimePtr(req.SnoozedUntil, nextSnooze) {
			req.SnoozedUntil = nextSnooze
			req.LastStateChangeAt = now
			addEvent("snoozed", map[string]any{"snoozed_until": nextSnooze})
			touchHuman = true
		}
	}

	if humanActor && touchHuman {
		req.LastHumanActivityAt = now
	}
	req.LastActivityAt = now
	ApplyRequestCompatibility(&req)

	if err := updateRequestTx(ctx, tx, req); err != nil {
		return Request{}, nil, err
	}

	policy, err := getQueuePolicyForRequestTx(ctx, tx, req)
	if err != nil {
		return Request{}, nil, err
	}
	if mutation.Reopen {
		if err := createClockPhaseTx(ctx, tx, req, policy, now); err != nil {
			return Request{}, nil, err
		}
	} else {
		if err := syncClockPhaseTx(ctx, tx, req, policy, now, queueChanged); err != nil {
			return Request{}, nil, err
		}
	}

	insertedEvents := make([]RequestEvent, 0, len(events))
	for _, event := range events {
		inserted, err := insertRequestEventTx(ctx, tx, event)
		if err != nil {
			return Request{}, nil, err
		}
		insertedEvents = append(insertedEvents, inserted)
	}

	if err := enqueueRefreshTx(ctx, tx, req.ID); err != nil {
		return Request{}, nil, err
	}
	if trackerSyncNeeded {
		if err := enqueueExternalSyncTx(ctx, tx, req.ID, "request_mutation"); err != nil {
			return Request{}, nil, err
		}
	}
	if linkTrackerRequested {
		if err := enqueueTrackerLinkTx(ctx, tx, req.ID, mutation.ActorID); err != nil {
			return Request{}, nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Request{}, nil, err
	}
	ApplyRequestCompatibility(&req)
	return req, insertedEvents, nil
}

func getRequestByIDTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID) (Request, error) {
	var req Request
	row := tx.QueryRow(ctx, fmt.Sprintf(`select %s from requests where id = $1`, requestSelectColumns("")), requestID)
	if err := scanRequest(row, &req); err != nil {
		return Request{}, err
	}
	return req, nil
}

func updateRequestTx(ctx context.Context, tx pgx.Tx, req Request) error {
	ApplyRequestCompatibility(&req)
	_, err := tx.Exec(ctx, `
		update requests
		set status = $2,
			priority = $3,
			owner_slack_id = $4,
			due_at = $5,
			acked_at = $6,
			assigned_at = $7,
			resolved_at = $8,
			last_activity_at = $9,
			state = $10,
			acknowledged_at = $11,
			acknowledged_by = $12,
			owner_user_id = $13,
			request_type = $14,
			queue_id = $15,
			waiting_on = $16,
			snoozed_until = $17,
			closed_at = $18,
			closed_by = $19,
			closed_reason = $20,
			last_human_activity_at = $21,
			last_state_change_at = $22
		where id = $1
	`, req.ID, req.Status, req.Priority, req.OwnerSlackID, req.DueAt, req.AckedAt, req.AssignedAt, req.ResolvedAt, req.LastActivityAt, req.State, req.AcknowledgedAt, req.AcknowledgedBy, req.OwnerUserID, req.RequestType, req.QueueID, req.WaitingOn, req.SnoozedUntil, req.ClosedAt, req.ClosedBy, req.ClosedReason, req.LastHumanActivityAt, req.LastStateChangeAt)
	return err
}

func getQueuePolicyForRequestTx(ctx context.Context, tx pgx.Tx, req Request) (QueuePolicy, error) {
	if req.QueueID != nil {
		var policy QueuePolicy
		var businessStart *string
		var businessEnd *string
		err := tx.QueryRow(ctx, `
			select queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
				to_char(digest_time, 'HH24:MI:SS'), digest_weekday, assign_starts_from, stale_starts_from,
				timezone, business_hours_enabled,
				case when business_hours_start is null then null else to_char(business_hours_start, 'HH24:MI:SS') end,
				case when business_hours_end is null then null else to_char(business_hours_end, 'HH24:MI:SS') end,
				business_days_mask, created_at, updated_at
			from queue_policies
			where queue_id = $1
		`, req.QueueID).Scan(
			&policy.QueueID,
			&policy.AckSLAMinutes,
			&policy.AssignSLAMinutes,
			&policy.StaleHours,
			&policy.DigestTime,
			&policy.DigestWeekday,
			&policy.AssignStartsFrom,
			&policy.StaleStartsFrom,
			&policy.Timezone,
			&policy.BusinessHoursEnabled,
			&businessStart,
			&businessEnd,
			&policy.BusinessDaysMask,
			&policy.CreatedAt,
			&policy.UpdatedAt,
		)
		if err == nil {
			policy.BusinessHoursStart = businessStart
			policy.BusinessHoursEnd = businessEnd
			return policy, nil
		}
		if err != nil && err != pgx.ErrNoRows {
			return QueuePolicy{}, err
		}
	}

	var policy QueuePolicy
	err := tx.QueryRow(ctx, `
		select q.id, p.ack_sla_minutes, p.assign_sla_minutes, p.stale_hours,
			to_char(p.daily_digest_time, 'HH24:MI:SS'), null, 'created_at', 'last_human_activity_at',
			coalesce(p.timezone, 'UTC'), false, null, null, 62, now(), now()
		from queues q
		join policies p on p.workspace_id = q.workspace_id
		where q.workspace_id = $1 and q.is_default = true
		order by q.created_at asc
		limit 1
	`, req.WorkspaceID).Scan(
		&policy.QueueID,
		&policy.AckSLAMinutes,
		&policy.AssignSLAMinutes,
		&policy.StaleHours,
		&policy.DigestTime,
		&policy.DigestWeekday,
		&policy.AssignStartsFrom,
		&policy.StaleStartsFrom,
		&policy.Timezone,
		&policy.BusinessHoursEnabled,
		&policy.BusinessHoursStart,
		&policy.BusinessHoursEnd,
		&policy.BusinessDaysMask,
		&policy.CreatedAt,
		&policy.UpdatedAt,
	)
	return policy, err
}

func createClockPhaseTx(ctx context.Context, tx pgx.Tx, req Request, policy QueuePolicy, now time.Time) error {
	for _, clockType := range []string{"ack", "assign", "stale"} {
		startedAt, targetAt, pausedAt, satisfiedAt, state := desiredClockState(req, policy, clockType, now)
		_, err := tx.Exec(ctx, `
			insert into request_sla_clocks (
				request_id, clock_type, started_at, target_at, paused_at,
				satisfied_at, breached_at, last_notified_at, state, created_at
			) values ($1, $2, $3, $4, $5, $6, null, null, $7, now())
		`, req.ID, clockType, startedAt, targetAt, pausedAt, satisfiedAt, state)
		if err != nil {
			return err
		}
	}
	return nil
}

func syncClockPhaseTx(ctx context.Context, tx pgx.Tx, req Request, policy QueuePolicy, now time.Time, resetTarget bool) error {
	for _, clockType := range []string{"ack", "assign", "stale"} {
		var clock RequestSLAClock
		err := tx.QueryRow(ctx, `
			select id, request_id, clock_type, started_at, target_at, paused_at,
				satisfied_at, breached_at, last_notified_at, state, created_at
			from request_sla_clocks
			where request_id = $1 and clock_type = $2
			order by created_at desc
			limit 1
		`, req.ID, clockType).Scan(
			&clock.ID,
			&clock.RequestID,
			&clock.ClockType,
			&clock.StartedAt,
			&clock.TargetAt,
			&clock.PausedAt,
			&clock.SatisfiedAt,
			&clock.BreachedAt,
			&clock.LastNotifiedAt,
			&clock.State,
			&clock.CreatedAt,
		)
		if err != nil {
			if err == pgx.ErrNoRows {
				if err := createClockPhaseTx(ctx, tx, req, policy, now); err != nil {
					return err
				}
				break
			}
			return err
		}

		startedAt, targetAt, pausedAt, satisfiedAt, state := desiredClockState(req, policy, clockType, now)
		if resetTarget {
			clock.StartedAt = startedAt
			clock.TargetAt = targetAt
		}
		if clockType == "stale" && req.LastHumanActivityAt.After(clock.StartedAt) {
			clock.StartedAt = req.LastHumanActivityAt
			clock.TargetAt = computeClockTarget(clock.StartedAt, clockType, policy)
			clock.BreachedAt = nil
			clock.LastNotifiedAt = nil
		}
		if clockType == "assign" && strings.EqualFold(policy.AssignStartsFrom, "acknowledged_at") && req.AcknowledgedAt != nil && clock.TargetAt == nil {
			clock.StartedAt = *req.AcknowledgedAt
			clock.TargetAt = computeClockTarget(clock.StartedAt, clockType, policy)
		}
		if clock.PausedAt != nil && pausedAt == nil && clock.TargetAt != nil {
			resumeShift := now.Sub(*clock.PausedAt)
			shifted := clock.TargetAt.Add(resumeShift)
			clock.TargetAt = &shifted
		}
		clock.PausedAt = pausedAt
		clock.SatisfiedAt = satisfiedAt
		if clock.BreachedAt != nil && (state == "running" || state == "paused") {
			clock.BreachedAt = nil
			clock.LastNotifiedAt = nil
		}
		if state == "satisfied" || state == "stopped" {
			clock.State = state
		} else {
			clock.State = state
		}
		if targetAt != nil && (clock.TargetAt == nil || resetTarget) {
			clock.TargetAt = targetAt
		}
		_, err = tx.Exec(ctx, `
			update request_sla_clocks
			set started_at = $2,
				target_at = $3,
				paused_at = $4,
				satisfied_at = $5,
				breached_at = $6,
				last_notified_at = $7,
				state = $8
			where id = $1
		`, clock.ID, clock.StartedAt, clock.TargetAt, clock.PausedAt, clock.SatisfiedAt, clock.BreachedAt, clock.LastNotifiedAt, clock.State)
		if err != nil {
			return err
		}
	}
	return nil
}

func desiredClockState(req Request, policy QueuePolicy, clockType string, now time.Time) (time.Time, *time.Time, *time.Time, *time.Time, string) {
	state := "running"
	var startedAt time.Time
	var targetAt *time.Time
	var pausedAt *time.Time
	var satisfiedAt *time.Time

	switch clockType {
	case "ack":
		startedAt = req.CreatedAt
		targetAt = computeClockTarget(startedAt, clockType, policy)
		if req.AcknowledgedAt != nil {
			satisfiedAt = req.AcknowledgedAt
			state = "satisfied"
		}
	case "assign":
		startedAt = req.CreatedAt
		if strings.EqualFold(strings.TrimSpace(policy.AssignStartsFrom), "acknowledged_at") {
			if req.AcknowledgedAt == nil {
				pausedAt = &now
				state = "paused"
				break
			}
			startedAt = *req.AcknowledgedAt
		}
		targetAt = computeClockTarget(startedAt, clockType, policy)
		if req.OwnerUserID != nil && strings.TrimSpace(*req.OwnerUserID) != "" {
			if req.AssignedAt != nil {
				satisfiedAt = req.AssignedAt
			} else {
				satisfiedAt = &req.LastStateChangeAt
			}
			state = "satisfied"
		}
	case "stale":
		startedAt = req.LastHumanActivityAt
		if startedAt.IsZero() {
			startedAt = req.CreatedAt
		}
		targetAt = computeClockTarget(startedAt, clockType, policy)
		if NormalizeRequestState(req.State) == RequestStateClosed {
			state = "stopped"
			break
		}
		if NormalizeWaitingOn(req.WaitingOn) != WaitingOnNone || (req.SnoozedUntil != nil && req.SnoozedUntil.After(now)) {
			pausedAt = &now
			state = "paused"
		}
	}

	if NormalizeRequestState(req.State) == RequestStateClosed && state != "satisfied" {
		state = "stopped"
	}

	return startedAt, targetAt, pausedAt, satisfiedAt, state
}

func computeClockTarget(start time.Time, clockType string, policy QueuePolicy) *time.Time {
	if start.IsZero() {
		return nil
	}
	var duration time.Duration
	switch clockType {
	case "ack":
		duration = time.Duration(policy.AckSLAMinutes) * time.Minute
	case "assign":
		duration = time.Duration(policy.AssignSLAMinutes) * time.Minute
	case "stale":
		duration = time.Duration(policy.StaleHours) * time.Hour
	default:
		return nil
	}
	target := addDurationWithBusinessHours(start, duration, policy)
	return &target
}

func addDurationWithBusinessHours(start time.Time, duration time.Duration, policy QueuePolicy) time.Time {
	if duration <= 0 {
		return start
	}
	if !policy.BusinessHoursEnabled || policy.BusinessHoursStart == nil || policy.BusinessHoursEnd == nil {
		return start.Add(duration)
	}

	loc, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		loc = time.UTC
	}
	current := start.In(loc)
	remaining := duration
	for remaining > 0 {
		windowStart, windowEnd := businessWindowForDay(current, policy, loc)
		if windowStart.IsZero() || windowEnd.IsZero() {
			current = beginningOfNextDay(current)
			continue
		}
		if current.Before(windowStart) {
			current = windowStart
		}
		if !current.Before(windowEnd) {
			current = beginningOfNextDay(current)
			continue
		}
		available := windowEnd.Sub(current)
		if available >= remaining {
			return current.Add(remaining).UTC()
		}
		remaining -= available
		current = beginningOfNextDay(current)
	}
	return current.UTC()
}

func businessWindowForDay(current time.Time, policy QueuePolicy, loc *time.Location) (time.Time, time.Time) {
	if !isBusinessDay(current, policy.BusinessDaysMask) {
		return time.Time{}, time.Time{}
	}
	startTOD, err := time.ParseInLocation("15:04:05", *policy.BusinessHoursStart, loc)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	endTOD, err := time.ParseInLocation("15:04:05", *policy.BusinessHoursEnd, loc)
	if err != nil {
		return time.Time{}, time.Time{}
	}
	windowStart := time.Date(current.Year(), current.Month(), current.Day(), startTOD.Hour(), startTOD.Minute(), startTOD.Second(), 0, loc)
	windowEnd := time.Date(current.Year(), current.Month(), current.Day(), endTOD.Hour(), endTOD.Minute(), endTOD.Second(), 0, loc)
	return windowStart, windowEnd
}

func isBusinessDay(current time.Time, mask int) bool {
	bit := 1 << int(current.Weekday())
	return mask&bit != 0
}

func beginningOfNextDay(current time.Time) time.Time {
	next := current.Add(24 * time.Hour)
	return time.Date(next.Year(), next.Month(), next.Day(), 0, 0, 0, 0, next.Location())
}

func insertRequestEventTx(ctx context.Context, tx pgx.Tx, event RequestEvent) (RequestEvent, error) {
	if len(event.Payload) == 0 {
		event.Payload = []byte(`{}`)
	}
	if strings.TrimSpace(event.ActorType) == "" {
		event.ActorType = "system"
	}
	var inserted RequestEvent
	err := tx.QueryRow(ctx, `
		insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
		values ($1, $2, $3, $4, $5, $6::jsonb)
		returning id, request_id, event_type, actor_type, actor_id, occurred_at, payload_json
	`, event.RequestID, event.EventType, event.ActorType, event.ActorID, event.OccurredAt, event.Payload).Scan(
		&inserted.ID,
		&inserted.RequestID,
		&inserted.EventType,
		&inserted.ActorType,
		&inserted.ActorID,
		&inserted.OccurredAt,
		&inserted.Payload,
	)
	if err != nil {
		return RequestEvent{}, err
	}
	return inserted, nil
}

func enqueueRefreshTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID) error {
	dedupeKey := fmt.Sprintf("triage_card_refresh:%s", requestID)
	payload := []byte(fmt.Sprintf(`{"request_id":"%s"}`, requestID))
	_, err := tx.Exec(ctx, `
		insert into notification_outbox (
			request_id, kind, destination_type, payload_json, dedupe_key, status, available_at, created_at, updated_at
		) values ($1, 'triage_card_refresh', 'request', $2::jsonb, $3, 'pending', now(), now(), now())
		on conflict (dedupe_key) where dedupe_key is not null
		do update set status = 'pending', available_at = now(), updated_at = now()
	`, requestID, payload, dedupeKey)
	return err
}

func enqueueExternalSyncTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID, source string) error {
	dedupeKey := fmt.Sprintf("external_sync:%s", requestID)
	payload := []byte(fmt.Sprintf(`{"request_id":"%s","source":"%s"}`, requestID, strings.TrimSpace(source)))
	_, err := tx.Exec(ctx, `
		insert into notification_outbox (
			request_id, kind, destination_type, payload_json, dedupe_key, status, available_at, created_at, updated_at
		) values ($1, 'external_sync_request', 'request', $2::jsonb, $3, 'pending', now(), now(), now())
		on conflict (dedupe_key) where dedupe_key is not null
		do update set status = 'pending', available_at = now(), updated_at = now()
	`, requestID, payload, dedupeKey)
	return err
}

func enqueueTrackerLinkTx(ctx context.Context, tx pgx.Tx, requestID uuid.UUID, actorID *string) error {
	dedupeKey := fmt.Sprintf("tracker_link:%s", requestID)
	payload := map[string]any{"request_id": requestID.String()}
	if actorID != nil {
		payload["actor_id"] = *actorID
	}
	data, _ := json.Marshal(payload)
	_, err := tx.Exec(ctx, `
		insert into notification_outbox (
			request_id, kind, destination_type, payload_json, dedupe_key, status, available_at, created_at, updated_at
		) values ($1, 'tracker_link_request', 'request', $2::jsonb, $3, 'pending', now(), now(), now())
		on conflict (dedupe_key) where dedupe_key is not null
		do update set status = 'pending', available_at = now(), updated_at = now()
	`, requestID, data, dedupeKey)
	return err
}

func fallbackActorType(value string) string {
	switch strings.TrimSpace(value) {
	case "slack_user", "system", "scheduler", "external_provider":
		return strings.TrimSpace(value)
	default:
		return "system"
	}
}

func normalizeCloseReason(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ClosedReasonResolved:
		return ClosedReasonResolved
	case ClosedReasonDuplicate:
		return ClosedReasonDuplicate
	case ClosedReasonNotPlanned:
		return ClosedReasonNotPlanned
	case ClosedReasonInvalid:
		return ClosedReasonInvalid
	default:
		return ClosedReasonNoise
	}
}

func sameTimePtr(left, right *time.Time) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.Equal(*right)
}
