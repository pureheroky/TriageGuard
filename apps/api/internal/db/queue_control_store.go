package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) ListQueues(ctx context.Context, workspaceID uuid.UUID) ([]Queue, error) {
	rows, err := s.pool.Query(ctx, `
		select id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
		from queues
		where workspace_id = $1
		order by is_default desc, name asc
	`, workspaceID)
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

func (s *Store) GetQueueByID(ctx context.Context, queueID uuid.UUID) (Queue, error) {
	var item Queue
	err := s.pool.QueryRow(ctx, `
		select id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
		from queues
		where id = $1
	`, queueID).Scan(
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
	)
	if err != nil {
		return Queue{}, err
	}
	return item, nil
}

func (s *Store) GetWorkspaceDefaultQueue(ctx context.Context, workspaceID uuid.UUID) (Queue, error) {
	var item Queue
	err := s.pool.QueryRow(ctx, `
		select id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
		from queues
		where workspace_id = $1 and is_default = true
		order by created_at asc
		limit 1
	`, workspaceID).Scan(
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
	)
	if err != nil {
		return Queue{}, err
	}
	return item, nil
}

func (s *Store) GetChannelDefaultQueue(ctx context.Context, workspaceID uuid.UUID, channelID string) (Queue, error) {
	var item Queue
	err := s.pool.QueryRow(ctx, `
		select q.id, q.workspace_id, q.name, q.description, q.default_priority, q.default_request_type,
			q.escalation_channel_id, q.is_default, q.created_at, q.updated_at
		from slack_channels sc
		join queues q on q.id = sc.default_queue_id
		where sc.workspace_id = $1 and sc.channel_id = $2
	`, workspaceID, strings.TrimSpace(channelID)).Scan(
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
	)
	if err != nil {
		return Queue{}, err
	}
	return item, nil
}

func (s *Store) UpsertQueue(ctx context.Context, queue Queue) (Queue, error) {
	var item Queue
	err := s.pool.QueryRow(ctx, `
		insert into queues (
			id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
		) values (
			coalesce($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, now(), now()
		)
		on conflict (id) do update set
			name = excluded.name,
			description = excluded.description,
			default_priority = excluded.default_priority,
			default_request_type = excluded.default_request_type,
			escalation_channel_id = excluded.escalation_channel_id,
			is_default = excluded.is_default,
			updated_at = now()
		returning id, workspace_id, name, description, default_priority, default_request_type,
			escalation_channel_id, is_default, created_at, updated_at
	`, uuidPtr(queue.ID), queue.WorkspaceID, strings.TrimSpace(queue.Name), queue.Description, normalizedPriority(queue.DefaultPriority), normalizeRequestType(queue.DefaultRequestType), queue.EscalationChannelID, queue.IsDefault).Scan(
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
	)
	if err != nil {
		return Queue{}, err
	}
	if item.IsDefault {
		_, err = s.pool.Exec(ctx, `
			update queues
			set is_default = false, updated_at = now()
			where workspace_id = $1 and id <> $2 and is_default = true
		`, item.WorkspaceID, item.ID)
		if err != nil {
			return Queue{}, err
		}
	}
	return item, nil
}

func (s *Store) SetChannelDefaultQueue(ctx context.Context, workspaceID uuid.UUID, channelID string, queueID *uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		update slack_channels
		set default_queue_id = $3
		where workspace_id = $1 and channel_id = $2
	`, workspaceID, strings.TrimSpace(channelID), queueID)
	return err
}

func (s *Store) GetQueuePolicy(ctx context.Context, queueID uuid.UUID) (QueuePolicy, error) {
	var item QueuePolicy
	var digestWeekday *int
	var businessStart *string
	var businessEnd *string
	err := s.pool.QueryRow(ctx, `
		select queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
			to_char(digest_time, 'HH24:MI:SS'), digest_weekday, assign_starts_from, stale_starts_from,
			timezone, business_hours_enabled,
			case when business_hours_start is null then null else to_char(business_hours_start, 'HH24:MI:SS') end,
			case when business_hours_end is null then null else to_char(business_hours_end, 'HH24:MI:SS') end,
			business_days_mask, created_at, updated_at
		from queue_policies
		where queue_id = $1
	`, queueID).Scan(
		&item.QueueID,
		&item.AckSLAMinutes,
		&item.AssignSLAMinutes,
		&item.StaleHours,
		&item.DigestTime,
		&digestWeekday,
		&item.AssignStartsFrom,
		&item.StaleStartsFrom,
		&item.Timezone,
		&item.BusinessHoursEnabled,
		&businessStart,
		&businessEnd,
		&item.BusinessDaysMask,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return QueuePolicy{}, err
	}
	item.DigestWeekday = digestWeekday
	item.BusinessHoursStart = businessStart
	item.BusinessHoursEnd = businessEnd
	return item, nil
}

func (s *Store) UpsertQueuePolicy(ctx context.Context, policy QueuePolicy) (QueuePolicy, error) {
	if policy.AckSLAMinutes <= 0 {
		policy.AckSLAMinutes = 15
	}
	if policy.AssignSLAMinutes <= 0 {
		policy.AssignSLAMinutes = 30
	}
	if policy.StaleHours <= 0 {
		policy.StaleHours = 24
	}
	if strings.TrimSpace(policy.DigestTime) == "" {
		policy.DigestTime = "09:00:00"
	}
	if strings.TrimSpace(policy.AssignStartsFrom) == "" {
		policy.AssignStartsFrom = "created_at"
	}
	if strings.TrimSpace(policy.StaleStartsFrom) == "" {
		policy.StaleStartsFrom = "last_human_activity_at"
	}
	if strings.TrimSpace(policy.Timezone) == "" {
		policy.Timezone = "UTC"
	}

	var item QueuePolicy
	var businessStart *string
	var businessEnd *string
	err := s.pool.QueryRow(ctx, `
		insert into queue_policies (
			queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
			digest_time, digest_weekday, assign_starts_from, stale_starts_from,
			timezone, business_hours_enabled, business_hours_start, business_hours_end,
			business_days_mask, created_at, updated_at
		) values (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::time, $12::time, $13, now(), now()
		)
		on conflict (queue_id) do update set
			ack_sla_minutes = excluded.ack_sla_minutes,
			assign_sla_minutes = excluded.assign_sla_minutes,
			stale_hours = excluded.stale_hours,
			digest_time = excluded.digest_time,
			digest_weekday = excluded.digest_weekday,
			assign_starts_from = excluded.assign_starts_from,
			stale_starts_from = excluded.stale_starts_from,
			timezone = excluded.timezone,
			business_hours_enabled = excluded.business_hours_enabled,
			business_hours_start = excluded.business_hours_start,
			business_hours_end = excluded.business_hours_end,
			business_days_mask = excluded.business_days_mask,
			updated_at = now()
		returning queue_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
			to_char(digest_time, 'HH24:MI:SS'), digest_weekday, assign_starts_from, stale_starts_from,
			timezone, business_hours_enabled,
			case when business_hours_start is null then null else to_char(business_hours_start, 'HH24:MI:SS') end,
			case when business_hours_end is null then null else to_char(business_hours_end, 'HH24:MI:SS') end,
			business_days_mask, created_at, updated_at
	`, policy.QueueID, policy.AckSLAMinutes, policy.AssignSLAMinutes, policy.StaleHours, policy.DigestTime, policy.DigestWeekday, policy.AssignStartsFrom, policy.StaleStartsFrom, policy.Timezone, policy.BusinessHoursEnabled, policy.BusinessHoursStart, policy.BusinessHoursEnd, policy.BusinessDaysMask).Scan(
		&item.QueueID,
		&item.AckSLAMinutes,
		&item.AssignSLAMinutes,
		&item.StaleHours,
		&item.DigestTime,
		&item.DigestWeekday,
		&item.AssignStartsFrom,
		&item.StaleStartsFrom,
		&item.Timezone,
		&item.BusinessHoursEnabled,
		&businessStart,
		&businessEnd,
		&item.BusinessDaysMask,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return QueuePolicy{}, err
	}
	item.BusinessHoursStart = businessStart
	item.BusinessHoursEnd = businessEnd
	return item, nil
}

func (s *Store) ListQueuePolicies(ctx context.Context, workspaceID uuid.UUID) ([]QueuePolicy, error) {
	rows, err := s.pool.Query(ctx, `
		select qp.queue_id, qp.ack_sla_minutes, qp.assign_sla_minutes, qp.stale_hours,
			to_char(qp.digest_time, 'HH24:MI:SS'), qp.digest_weekday, qp.assign_starts_from, qp.stale_starts_from,
			qp.timezone, qp.business_hours_enabled,
			case when qp.business_hours_start is null then null else to_char(qp.business_hours_start, 'HH24:MI:SS') end,
			case when qp.business_hours_end is null then null else to_char(qp.business_hours_end, 'HH24:MI:SS') end,
			qp.business_days_mask, qp.created_at, qp.updated_at
		from queue_policies qp
		join queues q on q.id = qp.queue_id
		where q.workspace_id = $1
		order by q.is_default desc, q.name asc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]QueuePolicy, 0)
	for rows.Next() {
		var item QueuePolicy
		var businessStart *string
		var businessEnd *string
		if err := rows.Scan(
			&item.QueueID,
			&item.AckSLAMinutes,
			&item.AssignSLAMinutes,
			&item.StaleHours,
			&item.DigestTime,
			&item.DigestWeekday,
			&item.AssignStartsFrom,
			&item.StaleStartsFrom,
			&item.Timezone,
			&item.BusinessHoursEnabled,
			&businessStart,
			&businessEnd,
			&item.BusinessDaysMask,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.BusinessHoursStart = businessStart
		item.BusinessHoursEnd = businessEnd
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SyncWorkspacePolicyToQueues(ctx context.Context, workspaceID uuid.UUID, policy Policy) error {
	queues, err := s.ListQueues(ctx, workspaceID)
	if err != nil {
		return err
	}
	for _, queue := range queues {
		if _, err := s.UpsertQueuePolicy(ctx, QueuePolicy{
			QueueID:              queue.ID,
			AckSLAMinutes:        policy.AckSLAMinutes,
			AssignSLAMinutes:     policy.AssignSLAMinutes,
			StaleHours:           policy.StaleHours,
			DigestTime:           policy.DailyDigestTime,
			DigestWeekday:        nil,
			AssignStartsFrom:     "created_at",
			StaleStartsFrom:      "last_human_activity_at",
			Timezone:             policy.Timezone,
			BusinessHoursEnabled: false,
			BusinessHoursStart:   nil,
			BusinessHoursEnd:     nil,
			BusinessDaysMask:     62,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListQueueMembers(ctx context.Context, queueID uuid.UUID) ([]QueueMember, error) {
	rows, err := s.pool.Query(ctx, `
		select queue_id, user_id, role, created_at, updated_at
		from queue_members
		where queue_id = $1
		order by role asc, user_id asc
	`, queueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]QueueMember, 0)
	for rows.Next() {
		var item QueueMember
		if err := rows.Scan(&item.QueueID, &item.UserID, &item.Role, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceQueueMembers(ctx context.Context, queueID uuid.UUID, members []QueueMember) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from queue_members where queue_id = $1`, queueID); err != nil {
		return err
	}
	for _, member := range members {
		role := strings.ToLower(strings.TrimSpace(member.Role))
		if role != "manager" {
			role = "triager"
		}
		userID := strings.TrimSpace(member.UserID)
		if userID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			insert into queue_members (queue_id, user_id, role, created_at, updated_at)
			values ($1, $2, $3, now(), now())
		`, queueID, userID, role); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ListQueueRoutingRules(ctx context.Context, workspaceID uuid.UUID) ([]QueueRoutingRule, error) {
	rows, err := s.pool.Query(ctx, `
		select id, workspace_id, name, sort_order, is_enabled, conditions_json, queue_id,
			request_type, priority, stop_processing, created_at, updated_at
		from queue_routing_rules
		where workspace_id = $1
		order by sort_order asc, created_at asc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]QueueRoutingRule, 0)
	for rows.Next() {
		var item QueueRoutingRule
		if err := rows.Scan(
			&item.ID,
			&item.WorkspaceID,
			&item.Name,
			&item.SortOrder,
			&item.IsEnabled,
			&item.ConditionsJSON,
			&item.QueueID,
			&item.RequestType,
			&item.Priority,
			&item.StopProcessing,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListEscalationStepsByQueue(ctx context.Context, queueID uuid.UUID) ([]EscalationStep, error) {
	rows, err := s.pool.Query(ctx, `
		select id, queue_id, clock_type, delay_minutes_after_breach, target_type, target_value,
			message_template, is_enabled, created_at, updated_at
		from escalation_steps
		where queue_id = $1
		order by clock_type asc, delay_minutes_after_breach asc, created_at asc
	`, queueID)
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

func (s *Store) ReplaceQueueRoutingRules(ctx context.Context, workspaceID uuid.UUID, rules []QueueRoutingRule) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from queue_routing_rules where workspace_id = $1`, workspaceID); err != nil {
		return err
	}
	for idx, rule := range rules {
		if len(rule.ConditionsJSON) == 0 {
			rule.ConditionsJSON = []byte(`{}`)
		}
		name := strings.TrimSpace(rule.Name)
		if name == "" {
			name = fmt.Sprintf("Rule %d", idx+1)
		}
		if _, err := tx.Exec(ctx, `
			insert into queue_routing_rules (
				id, workspace_id, name, sort_order, is_enabled, conditions_json, queue_id,
				request_type, priority, stop_processing, created_at, updated_at
			) values (
				coalesce($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, now(), now()
			)
		`, uuidPtr(rule.ID), workspaceID, name, rule.SortOrder, rule.IsEnabled, rule.ConditionsJSON, rule.QueueID, normalizeOptionalString(rule.RequestType), normalizeOptionalPriority(rule.Priority), rule.StopProcessing); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ReplaceEscalationSteps(ctx context.Context, queueID uuid.UUID, steps []EscalationStep) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from escalation_steps where queue_id = $1`, queueID); err != nil {
		return err
	}
	for _, step := range steps {
		clockType := strings.TrimSpace(step.ClockType)
		switch clockType {
		case "ack", "assign", "stale":
		default:
			clockType = "ack"
		}
		targetType := strings.TrimSpace(step.TargetType)
		switch targetType {
		case "thread", "channel", "dm_user":
		default:
			targetType = "thread"
		}
		if _, err := tx.Exec(ctx, `
			insert into escalation_steps (
				id, queue_id, clock_type, delay_minutes_after_breach, target_type,
				target_value, message_template, is_enabled, created_at, updated_at
			) values (
				coalesce($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, now(), now()
			)
		`, uuidPtr(step.ID), queueID, clockType, step.DelayMinutesAfterBreach, targetType, normalizeOptionalString(step.TargetValue), normalizeOptionalString(step.MessageTemplate), step.IsEnabled); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) ResolveIntakeRouting(ctx context.Context, workspaceID uuid.UUID, channelID, authorSlackID, messageText string) (Queue, string, string, error) {
	rules, err := s.ListQueueRoutingRules(ctx, workspaceID)
	if err != nil {
		return Queue{}, "", "", err
	}
	for _, rule := range rules {
		if !rule.IsEnabled {
			continue
		}
		if !matchRoutingRule(rule.ConditionsJSON, channelID, authorSlackID, messageText) {
			continue
		}
		queue, err := s.GetQueueByID(ctx, rule.QueueID)
		if err != nil {
			return Queue{}, "", "", err
		}
		requestType := queue.DefaultRequestType
		if rule.RequestType != nil && strings.TrimSpace(*rule.RequestType) != "" {
			requestType = normalizeRequestType(*rule.RequestType)
		}
		priority := queue.DefaultPriority
		if rule.Priority != nil && strings.TrimSpace(*rule.Priority) != "" {
			priority = normalizedPriority(*rule.Priority)
		}
		return queue, requestType, priority, nil
	}

	queue, err := s.GetChannelDefaultQueue(ctx, workspaceID, channelID)
	if err == nil {
		return queue, queue.DefaultRequestType, queue.DefaultPriority, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Queue{}, "", "", err
	}

	queue, err = s.GetWorkspaceDefaultQueue(ctx, workspaceID)
	if err != nil {
		return Queue{}, "", "", err
	}
	return queue, queue.DefaultRequestType, queue.DefaultPriority, nil
}

func matchRoutingRule(raw json.RawMessage, channelID, authorSlackID, messageText string) bool {
	if len(raw) == 0 {
		return false
	}
	var node RoutingRuleNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return false
	}
	return evaluateRoutingNode(node, channelID, authorSlackID, messageText)
}

func evaluateRoutingNode(node RoutingRuleNode, channelID, authorSlackID, messageText string) bool {
	matchMode := strings.ToLower(strings.TrimSpace(node.Match))
	if len(node.Conditions) > 0 {
		if matchMode == "any" {
			for _, child := range node.Conditions {
				if evaluateRoutingNode(child, channelID, authorSlackID, messageText) {
					return true
				}
			}
			return false
		}
		for _, child := range node.Conditions {
			if !evaluateRoutingNode(child, channelID, authorSlackID, messageText) {
				return false
			}
		}
		return true
	}

	field := strings.ToLower(strings.TrimSpace(node.Field))
	operator := strings.ToLower(strings.TrimSpace(node.Operator))
	if field == "" || operator == "" {
		return false
	}

	var candidate string
	switch field {
	case "channel_id":
		candidate = strings.TrimSpace(channelID)
	case "author_slack_id":
		candidate = strings.TrimSpace(authorSlackID)
	case "message_text":
		candidate = messageText
	default:
		return false
	}

	switch operator {
	case "equals":
		value, _ := node.Value.(string)
		return strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(value))
	case "contains":
		value, _ := node.Value.(string)
		return value != "" && strings.Contains(strings.ToLower(candidate), strings.ToLower(value))
	case "starts_with":
		value, _ := node.Value.(string)
		return value != "" && strings.HasPrefix(strings.ToLower(candidate), strings.ToLower(value))
	case "regex":
		value, _ := node.Value.(string)
		if strings.TrimSpace(value) == "" {
			return false
		}
		re, err := regexp.Compile(value)
		if err != nil {
			return false
		}
		return re.MatchString(candidate)
	case "in":
		switch typed := node.Value.(type) {
		case []any:
			for _, item := range typed {
				if value, ok := item.(string); ok && strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(value)) {
					return true
				}
			}
		case []string:
			for _, value := range typed {
				if strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(value)) {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizedPriority(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "P0":
		return "P0"
	case "P1":
		return "P1"
	default:
		return "P2"
	}
}

func normalizeOptionalPriority(value *string) *string {
	if value == nil {
		return nil
	}
	normalized := normalizedPriority(*value)
	return &normalized
}

func normalizeRequestType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "bug":
		return "bug"
	case "incident":
		return "incident"
	case "access":
		return "access"
	case "infra":
		return "infra"
	case "question":
		return "question"
	case "task":
		return "task"
	default:
		return "other"
	}
}

func (s *Store) ListExternalIssuesByRequest(ctx context.Context, requestID uuid.UUID) ([]ExternalIssue, error) {
	rows, err := s.pool.Query(ctx, `
		select id, request_id, provider, external_id, external_key, external_url,
			sync_state, last_sync_at, last_sync_error, created_at, updated_at
		from external_issues
		where request_id = $1
		order by provider asc, created_at asc
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ExternalIssue, 0)
	for rows.Next() {
		var item ExternalIssue
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.Provider,
			&item.ExternalID,
			&item.ExternalKey,
			&item.ExternalURL,
			&item.SyncState,
			&item.LastSyncAt,
			&item.LastSyncError,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) UpsertExternalIssue(ctx context.Context, issue ExternalIssue) (ExternalIssue, error) {
	var item ExternalIssue
	err := s.pool.QueryRow(ctx, `
		insert into external_issues (
			id, request_id, provider, external_id, external_key, external_url,
			sync_state, last_sync_at, last_sync_error, created_at, updated_at
		) values (
			coalesce($1::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, now(), now()
		)
		on conflict (request_id, provider) do update set
			external_id = excluded.external_id,
			external_key = excluded.external_key,
			external_url = excluded.external_url,
			sync_state = excluded.sync_state,
			last_sync_at = excluded.last_sync_at,
			last_sync_error = excluded.last_sync_error,
			updated_at = now()
		returning id, request_id, provider, external_id, external_key, external_url,
			sync_state, last_sync_at, last_sync_error, created_at, updated_at
	`, uuidPtr(issue.ID), issue.RequestID, strings.ToLower(strings.TrimSpace(issue.Provider)), strings.TrimSpace(issue.ExternalID), issue.ExternalKey, issue.ExternalURL, strings.TrimSpace(issue.SyncState), issue.LastSyncAt, issue.LastSyncError).Scan(
		&item.ID,
		&item.RequestID,
		&item.Provider,
		&item.ExternalID,
		&item.ExternalKey,
		&item.ExternalURL,
		&item.SyncState,
		&item.LastSyncAt,
		&item.LastSyncError,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return ExternalIssue{}, err
	}
	return item, nil
}

func (s *Store) GetExternalIssueByProvider(ctx context.Context, requestID uuid.UUID, provider string) (ExternalIssue, error) {
	var item ExternalIssue
	err := s.pool.QueryRow(ctx, `
		select id, request_id, provider, external_id, external_key, external_url,
			sync_state, last_sync_at, last_sync_error, created_at, updated_at
		from external_issues
		where request_id = $1 and provider = $2
	`, requestID, strings.ToLower(strings.TrimSpace(provider))).Scan(
		&item.ID,
		&item.RequestID,
		&item.Provider,
		&item.ExternalID,
		&item.ExternalKey,
		&item.ExternalURL,
		&item.SyncState,
		&item.LastSyncAt,
		&item.LastSyncError,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return ExternalIssue{}, err
	}
	return item, nil
}

func (s *Store) InsertRequestEvent(ctx context.Context, event RequestEvent) (RequestEvent, error) {
	if len(event.Payload) == 0 {
		event.Payload = []byte(`{}`)
	}
	if strings.TrimSpace(event.ActorType) == "" {
		event.ActorType = "system"
	}
	var inserted RequestEvent
	err := s.pool.QueryRow(ctx, `
		insert into request_events (request_id, event_type, actor_type, actor_id, occurred_at, payload_json)
		values ($1, $2, $3, $4, coalesce($5, now()), $6::jsonb)
		returning id, request_id, event_type, actor_type, actor_id, occurred_at, payload_json
	`, event.RequestID, strings.TrimSpace(event.EventType), strings.TrimSpace(event.ActorType), event.ActorID, nullableTime(event.OccurredAt), event.Payload).Scan(
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

func (s *Store) InsertOutboxItem(ctx context.Context, item NotificationOutboxItem) (NotificationOutboxItem, error) {
	if len(item.Payload) == 0 {
		item.Payload = []byte(`{}`)
	}
	var inserted NotificationOutboxItem
	err := s.pool.QueryRow(ctx, `
		insert into notification_outbox (
			request_id, event_id, kind, destination_type, destination_value, payload_json,
			dedupe_key, status, retry_count, available_at, sent_at, failed_at, last_error, created_at, updated_at
		) values (
			$1, $2, $3, $4, $5, $6::jsonb, $7, coalesce($8, 'pending'), coalesce($9, 0),
			coalesce($10, now()), $11, $12, $13, now(), now()
		)
		on conflict (dedupe_key) where dedupe_key is not null
		do update set updated_at = now()
		returning id, request_id, event_id, kind, destination_type, destination_value, payload_json,
			dedupe_key, status, retry_count, available_at, sent_at, failed_at, last_error, created_at, updated_at
	`, item.RequestID, item.EventID, strings.TrimSpace(item.Kind), strings.TrimSpace(item.DestinationType), item.DestinationValue, item.Payload, item.DedupeKey, strings.TrimSpace(item.Status), item.RetryCount, nullableTime(item.AvailableAt), item.SentAt, item.FailedAt, item.LastError).Scan(
		&inserted.ID,
		&inserted.RequestID,
		&inserted.EventID,
		&inserted.Kind,
		&inserted.DestinationType,
		&inserted.DestinationValue,
		&inserted.Payload,
		&inserted.DedupeKey,
		&inserted.Status,
		&inserted.RetryCount,
		&inserted.AvailableAt,
		&inserted.SentAt,
		&inserted.FailedAt,
		&inserted.LastError,
		&inserted.CreatedAt,
		&inserted.UpdatedAt,
	)
	if err != nil {
		return NotificationOutboxItem{}, err
	}
	return inserted, nil
}

func (s *Store) ListRequestSLAClocks(ctx context.Context, requestID uuid.UUID) ([]RequestSLAClock, error) {
	rows, err := s.pool.Query(ctx, `
		select distinct on (clock_type)
			id, request_id, clock_type, started_at, target_at, paused_at,
			satisfied_at, breached_at, last_notified_at, state, created_at
		from request_sla_clocks
		where request_id = $1
		order by clock_type asc, created_at desc
	`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RequestSLAClock, 0)
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
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListRequestEvents(ctx context.Context, requestID uuid.UUID, limit int) ([]RequestEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select id, request_id, event_type, actor_type, actor_id, occurred_at, payload_json
		from request_events
		where request_id = $1
		order by occurred_at desc
		limit $2
	`, requestID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RequestEvent, 0, limit)
	for rows.Next() {
		var item RequestEvent
		if err := rows.Scan(&item.ID, &item.RequestID, &item.EventType, &item.ActorType, &item.ActorID, &item.OccurredAt, &item.Payload); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) QueueDashboard(ctx context.Context, workspaceID uuid.UUID) (QueueDashboardSummary, []QueueSummaryRow, []QueueSummaryRow, []QueueSummaryRow, error) {
	queues, err := s.ListQueues(ctx, workspaceID)
	if err != nil {
		return QueueDashboardSummary{}, nil, nil, nil, err
	}
	policies, err := s.ListQueuePolicies(ctx, workspaceID)
	if err != nil {
		return QueueDashboardSummary{}, nil, nil, nil, err
	}
	policyByQueue := make(map[uuid.UUID]QueuePolicy, len(policies))
	for _, policy := range policies {
		policyByQueue[policy.QueueID] = policy
	}

	type aggregate struct {
		row           QueueSummaryRow
		ackSamples    []float64
		assignSamples []float64
		closeSamples  []float64
		openAgeHours  []float64
		maxOpenAge    float64
	}
	aggByQueue := make(map[uuid.UUID]*aggregate, len(queues))
	for _, queue := range queues {
		row := QueueSummaryRow{Queue: queue}
		if policy, ok := policyByQueue[queue.ID]; ok {
			row.Policy = policy
		}
		aggByQueue[queue.ID] = &aggregate{row: row}
	}

	rows, err := s.pool.Query(ctx, `
		select
			r.id,
			r.queue_id,
			r.state,
			r.request_type,
			r.acknowledged_at,
			r.owner_user_id,
			r.waiting_on,
			r.snoozed_until,
			r.created_at,
			r.assigned_at,
			r.closed_at,
			r.last_human_activity_at,
			ack.started_at, ack.target_at, ack.state,
			assignc.started_at, assignc.target_at, assignc.state,
			stale.started_at, stale.target_at, stale.state
		from requests r
		left join lateral (
			select started_at, target_at, state
			from request_sla_clocks
			where request_id = r.id and clock_type = 'ack'
			order by created_at desc
			limit 1
		) ack on true
		left join lateral (
			select started_at, target_at, state
			from request_sla_clocks
			where request_id = r.id and clock_type = 'assign'
			order by created_at desc
			limit 1
		) assignc on true
		left join lateral (
			select started_at, target_at, state
			from request_sla_clocks
			where request_id = r.id and clock_type = 'stale'
			order by created_at desc
			limit 1
		) stale on true
		where r.workspace_id = $1
	`, workspaceID)
	if err != nil {
		return QueueDashboardSummary{}, nil, nil, nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	allAck := make([]float64, 0)
	allAssign := make([]float64, 0)
	allClose := make([]float64, 0)

	for rows.Next() {
		var requestID uuid.UUID
		var queueID *uuid.UUID
		var state string
		var requestType string
		var acknowledgedAt *time.Time
		var ownerUserID *string
		var waitingOn string
		var snoozedUntil *time.Time
		var createdAt time.Time
		var assignedAt *time.Time
		var closedAt *time.Time
		var lastHumanActivityAt time.Time
		var ackStarted, ackTarget *time.Time
		var ackState *string
		var assignStarted, assignTarget *time.Time
		var assignState *string
		var staleStarted, staleTarget *time.Time
		var staleState *string

		if err := rows.Scan(
			&requestID,
			&queueID,
			&state,
			&requestType,
			&acknowledgedAt,
			&ownerUserID,
			&waitingOn,
			&snoozedUntil,
			&createdAt,
			&assignedAt,
			&closedAt,
			&lastHumanActivityAt,
			&ackStarted,
			&ackTarget,
			&ackState,
			&assignStarted,
			&assignTarget,
			&assignState,
			&staleStarted,
			&staleTarget,
			&staleState,
		); err != nil {
			return QueueDashboardSummary{}, nil, nil, nil, err
		}
		if queueID == nil {
			continue
		}
		agg := aggByQueue[*queueID]
		if agg == nil {
			continue
		}

		isOpen := strings.EqualFold(state, RequestStateOpen)
		if isOpen {
			agg.row.OpenCount++
			openAgeHours := now.Sub(createdAt).Hours()
			if openAgeHours < 0 {
				openAgeHours = 0
			}
			agg.openAgeHours = append(agg.openAgeHours, openAgeHours)
			if openAgeHours > agg.maxOpenAge {
				agg.maxOpenAge = openAgeHours
			}
			if acknowledgedAt == nil {
				agg.row.UnackedCount++
			}
			if ownerUserID == nil || strings.TrimSpace(*ownerUserID) == "" {
				agg.row.UnassignedCount++
			}
			if NormalizeWaitingOn(waitingOn) != WaitingOnNone || (snoozedUntil != nil && snoozedUntil.After(now)) {
				agg.row.WaitingCount++
				if now.Sub(lastHumanActivityAt) >= 24*time.Hour {
					agg.row.WaitingDebtCount++
				}
			}
			if now.Sub(lastHumanActivityAt) >= 24*time.Hour {
				agg.row.NoHumanActivityCount++
			}
		}

		requestBreached := false
		requestAtRisk := false
		if clockIsBreachedState(ackState) || clockIsBreachedState(assignState) || clockIsBreachedState(staleState) {
			requestBreached = true
		}
		if clockIsAtRiskTarget(ackStarted, ackTarget, ackState, now) || clockIsAtRiskTarget(assignStarted, assignTarget, assignState, now) || clockIsAtRiskTarget(staleStarted, staleTarget, staleState, now) {
			requestAtRisk = true
		}
		if requestBreached {
			agg.row.BreachedCount++
		}
		if requestAtRisk {
			agg.row.AtRiskCount++
		}
		if clockIsBreachedState(staleState) {
			agg.row.BreachedStaleCount++
		}

		if acknowledgedAt != nil {
			sample := acknowledgedAt.Sub(createdAt).Minutes()
			agg.ackSamples = append(agg.ackSamples, sample)
			allAck = append(allAck, sample)
		}
		if assignedAt != nil {
			sample := assignedAt.Sub(createdAt).Minutes()
			agg.assignSamples = append(agg.assignSamples, sample)
			allAssign = append(allAssign, sample)
		}
		if closedAt != nil {
			sample := closedAt.Sub(createdAt).Minutes()
			agg.closeSamples = append(agg.closeSamples, sample)
			allClose = append(allClose, sample)
		}
	}
	if err := rows.Err(); err != nil {
		return QueueDashboardSummary{}, nil, nil, nil, err
	}

	queueRows := make([]QueueSummaryRow, 0, len(aggByQueue))
	for _, agg := range aggByQueue {
		agg.row.MedianAckMinutes = medianFloat(agg.ackSamples)
		agg.row.MedianAssignMinutes = medianFloat(agg.assignSamples)
		agg.row.MedianCloseMinutes = medianFloat(agg.closeSamples)
		queueRows = append(queueRows, agg.row)
	}
	sort.Slice(queueRows, func(i, j int) bool {
		if queueRows[i].BreachedCount == queueRows[j].BreachedCount {
			return queueRows[i].Queue.Name < queueRows[j].Queue.Name
		}
		return queueRows[i].BreachedCount > queueRows[j].BreachedCount
	})

	summary := QueueDashboardSummary{
		MedianAckMinutes:    medianFloat(allAck),
		MedianAssignMinutes: medianFloat(allAssign),
		MedianCloseMinutes:  medianFloat(allClose),
	}
	for _, row := range queueRows {
		summary.OpenCount += row.OpenCount
		summary.UnackedCount += row.UnackedCount
		summary.UnassignedCount += row.UnassignedCount
		summary.WaitingCount += row.WaitingCount
		summary.BreachedCount += row.BreachedCount
		summary.AtRiskCount += row.AtRiskCount
		summary.WaitingDebtCount += row.WaitingDebtCount
		summary.NoHumanActivityCount += row.NoHumanActivityCount
	}

	topBreached := cloneQueueRows(queueRows)
	sort.Slice(topBreached, func(i, j int) bool {
		if topBreached[i].BreachedCount == topBreached[j].BreachedCount {
			return topBreached[i].Queue.Name < topBreached[j].Queue.Name
		}
		return topBreached[i].BreachedCount > topBreached[j].BreachedCount
	})
	if len(topBreached) > 5 {
		topBreached = topBreached[:5]
	}

	topStale := cloneQueueRows(queueRows)
	sort.Slice(topStale, func(i, j int) bool {
		if topStale[i].BreachedStaleCount == topStale[j].BreachedStaleCount {
			return topStale[i].Queue.Name < topStale[j].Queue.Name
		}
		return topStale[i].BreachedStaleCount > topStale[j].BreachedStaleCount
	})
	if len(topStale) > 5 {
		topStale = topStale[:5]
	}

	return summary, queueRows, topBreached, topStale, nil
}

func (s *Store) QueueDashboardAnalytics(ctx context.Context, workspaceID uuid.UUID) (QueueDashboardAnalytics, error) {
	summary, queueRows, topBreached, topStale, err := s.QueueDashboard(ctx, workspaceID)
	if err != nil {
		return QueueDashboardAnalytics{}, err
	}

	type typeAggregate struct {
		row           TypeSummaryRow
		ackSamples    []float64
		assignSamples []float64
		closeSamples  []float64
	}
	typeAgg := map[string]*typeAggregate{}
	agingByQueue := make([]QueueAgingRow, 0, len(queueRows))
	now := time.Now().UTC()

	for _, row := range queueRows {
		var avgAge float64
		var maxAge float64
		var ageCount int
		requestRows, err := s.pool.Query(ctx, `
			select created_at
			from requests
			where queue_id = $1 and state = 'OPEN'
		`, row.Queue.ID)
		if err != nil {
			return QueueDashboardAnalytics{}, err
		}
		for requestRows.Next() {
			var createdAt time.Time
			if err := requestRows.Scan(&createdAt); err != nil {
				requestRows.Close()
				return QueueDashboardAnalytics{}, err
			}
			ageHours := now.Sub(createdAt).Hours()
			if ageHours < 0 {
				ageHours = 0
			}
			avgAge += ageHours
			ageCount++
			if ageHours > maxAge {
				maxAge = ageHours
			}
		}
		requestRows.Close()
		if ageCount > 0 {
			avgAge /= float64(ageCount)
		}
		agingByQueue = append(agingByQueue, QueueAgingRow{
			QueueID:         row.Queue.ID,
			QueueName:       row.Queue.Name,
			OpenCount:       row.OpenCount,
			AvgOpenAgeHours: avgAge,
			MaxOpenAgeHours: maxAge,
		})
	}

	rows, err := s.pool.Query(ctx, `
		select request_type, state, acknowledged_at, assigned_at, closed_at, created_at,
			waiting_on, snoozed_until, owner_user_id, last_human_activity_at,
			exists (
				select 1 from request_sla_clocks c
				where c.request_id = r.id and c.state = 'breached'
				order by created_at desc
				limit 1
			) as is_breached
		from requests r
		where workspace_id = $1
	`, workspaceID)
	if err != nil {
		return QueueDashboardAnalytics{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var requestType string
		var state string
		var acknowledgedAt *time.Time
		var assignedAt *time.Time
		var closedAt *time.Time
		var createdAt time.Time
		var waitingOn string
		var snoozedUntil *time.Time
		var ownerUserID *string
		var lastHumanActivityAt time.Time
		var isBreached bool
		if err := rows.Scan(&requestType, &state, &acknowledgedAt, &assignedAt, &closedAt, &createdAt, &waitingOn, &snoozedUntil, &ownerUserID, &lastHumanActivityAt, &isBreached); err != nil {
			return QueueDashboardAnalytics{}, err
		}
		requestType = normalizeRequestType(requestType)
		agg := typeAgg[requestType]
		if agg == nil {
			agg = &typeAggregate{row: TypeSummaryRow{RequestType: requestType}}
			typeAgg[requestType] = agg
		}
		if strings.EqualFold(state, RequestStateOpen) {
			agg.row.OpenCount++
			if NormalizeWaitingOn(waitingOn) != WaitingOnNone || (snoozedUntil != nil && snoozedUntil.After(now)) {
				agg.row.WaitingCount++
			}
			if ownerUserID == nil || strings.TrimSpace(*ownerUserID) == "" {
				agg.row.UnassignedCount++
			}
			if now.Sub(lastHumanActivityAt) >= 24*time.Hour {
				agg.row.NoHumanActivityCount++
			}
		}
		if isBreached {
			agg.row.BreachedCount++
		}
		if acknowledgedAt != nil {
			agg.ackSamples = append(agg.ackSamples, acknowledgedAt.Sub(createdAt).Minutes())
		}
		if assignedAt != nil {
			start := createdAt
			if acknowledgedAt != nil {
				start = *acknowledgedAt
			}
			agg.assignSamples = append(agg.assignSamples, assignedAt.Sub(start).Minutes())
		}
		if closedAt != nil {
			agg.closeSamples = append(agg.closeSamples, closedAt.Sub(createdAt).Minutes())
		}
	}
	if err := rows.Err(); err != nil {
		return QueueDashboardAnalytics{}, err
	}

	agingByType := make([]TypeSummaryRow, 0, len(typeAgg))
	for _, agg := range typeAgg {
		agg.row.MedianAckMinutes = medianFloat(agg.ackSamples)
		agg.row.MedianAssignMinutes = medianFloat(agg.assignSamples)
		agg.row.MedianCloseMinutes = medianFloat(agg.closeSamples)
		agingByType = append(agingByType, agg.row)
	}
	sort.Slice(agingByQueue, func(i, j int) bool {
		if agingByQueue[i].AvgOpenAgeHours == agingByQueue[j].AvgOpenAgeHours {
			return agingByQueue[i].QueueName < agingByQueue[j].QueueName
		}
		return agingByQueue[i].AvgOpenAgeHours > agingByQueue[j].AvgOpenAgeHours
	})
	sort.Slice(agingByType, func(i, j int) bool {
		if agingByType[i].OpenCount == agingByType[j].OpenCount {
			return agingByType[i].RequestType < agingByType[j].RequestType
		}
		return agingByType[i].OpenCount > agingByType[j].OpenCount
	})

	waitingDebt := cloneQueueRows(queueRows)
	sort.Slice(waitingDebt, func(i, j int) bool {
		if waitingDebt[i].WaitingDebtCount == waitingDebt[j].WaitingDebtCount {
			return waitingDebt[i].Queue.Name < waitingDebt[j].Queue.Name
		}
		return waitingDebt[i].WaitingDebtCount > waitingDebt[j].WaitingDebtCount
	})
	if len(waitingDebt) > 5 {
		waitingDebt = waitingDebt[:5]
	}
	unassignedDebt := cloneQueueRows(queueRows)
	sort.Slice(unassignedDebt, func(i, j int) bool {
		if unassignedDebt[i].UnassignedCount == unassignedDebt[j].UnassignedCount {
			return unassignedDebt[i].Queue.Name < unassignedDebt[j].Queue.Name
		}
		return unassignedDebt[i].UnassignedCount > unassignedDebt[j].UnassignedCount
	})
	if len(unassignedDebt) > 5 {
		unassignedDebt = unassignedDebt[:5]
	}
	noHuman := cloneQueueRows(queueRows)
	sort.Slice(noHuman, func(i, j int) bool {
		if noHuman[i].NoHumanActivityCount == noHuman[j].NoHumanActivityCount {
			return noHuman[i].Queue.Name < noHuman[j].Queue.Name
		}
		return noHuman[i].NoHumanActivityCount > noHuman[j].NoHumanActivityCount
	})
	if len(noHuman) > 5 {
		noHuman = noHuman[:5]
	}

	return QueueDashboardAnalytics{
		Summary:         summary,
		Queues:          queueRows,
		TopBreached:     topBreached,
		TopStale:        topStale,
		AgingByQueue:    agingByQueue,
		AgingByType:     agingByType,
		WaitingDebt:     waitingDebt,
		UnassignedDebt:  unassignedDebt,
		NoHumanActivity: noHuman,
	}, nil
}

func clockIsBreachedState(state *string) bool {
	return state != nil && strings.EqualFold(strings.TrimSpace(*state), "breached")
}

func clockIsAtRiskTarget(startedAt, targetAt *time.Time, state *string, now time.Time) bool {
	if startedAt == nil || targetAt == nil || state == nil || !strings.EqualFold(strings.TrimSpace(*state), "running") {
		return false
	}
	total := targetAt.Sub(*startedAt)
	if total <= 0 {
		return false
	}
	return now.Sub(*startedAt) >= time.Duration(float64(total)*0.8)
}

func medianFloat(samples []float64) *float64 {
	if len(samples) == 0 {
		return nil
	}
	sort.Float64s(samples)
	mid := len(samples) / 2
	var value float64
	if len(samples)%2 == 0 {
		value = (samples[mid-1] + samples[mid]) / 2
	} else {
		value = samples[mid]
	}
	return &value
}

func cloneQueueRows(rows []QueueSummaryRow) []QueueSummaryRow {
	out := make([]QueueSummaryRow, len(rows))
	copy(out, rows)
	return out
}

func (s *Store) GetQueueDetailMetrics(ctx context.Context, queueID uuid.UUID) (QueueDetailMetrics, error) {
	queue, err := s.GetQueueByID(ctx, queueID)
	if err != nil {
		return QueueDetailMetrics{}, err
	}
	policy, err := s.GetQueuePolicy(ctx, queueID)
	if err != nil {
		return QueueDetailMetrics{}, err
	}
	_, queueRows, _, _, err := s.QueueDashboard(ctx, queue.WorkspaceID)
	if err != nil {
		return QueueDetailMetrics{}, err
	}
	out := QueueDetailMetrics{
		Queue:        queue,
		Policy:       policy,
		CloseReasons: map[string]int{},
	}
	for _, row := range queueRows {
		if row.Queue.ID != queueID {
			continue
		}
		out.CreatedCount = row.OpenCount + row.BreachedCount // overwritten below by actual count
		out.UnackedCount = row.UnackedCount
		out.UnassignedCount = row.UnassignedCount
		out.WaitingCount = row.WaitingCount
		out.BreachedCount = row.BreachedCount
		out.AtRiskCount = row.AtRiskCount
		out.WaitingDebtCount = row.WaitingDebtCount
		out.NoHumanActivityCount = row.NoHumanActivityCount
		out.MedianAckMinutes = row.MedianAckMinutes
		out.MedianAssignMinutes = row.MedianAssignMinutes
		out.MedianCloseMinutes = row.MedianCloseMinutes
		break
	}

	if err := s.pool.QueryRow(ctx, `
		select count(*)
		from requests
		where queue_id = $1
	`, queueID).Scan(&out.CreatedCount); err != nil {
		return QueueDetailMetrics{}, err
	}

	rows, err := s.pool.Query(ctx, `
		select coalesce(closed_reason, 'open') as reason, count(*)
		from requests
		where queue_id = $1
		group by 1
	`, queueID)
	if err != nil {
		return QueueDetailMetrics{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var count int
		if err := rows.Scan(&reason, &count); err != nil {
			return QueueDetailMetrics{}, err
		}
		out.CloseReasons[reason] = count
	}
	if err := rows.Err(); err != nil {
		return QueueDetailMetrics{}, err
	}

	var reopenedCount int
	var totalCount int
	if err := s.pool.QueryRow(ctx, `
		select
			(select count(*) from request_events re join requests r on r.id = re.request_id where r.queue_id = $1 and re.event_type = 'request_reopened') as reopened_count,
			(select count(*) from requests where queue_id = $1) as total_count
	`, queueID).Scan(&reopenedCount, &totalCount); err != nil {
		return QueueDetailMetrics{}, err
	}
	if totalCount > 0 {
		out.ReopenRate = float64(reopenedCount) / float64(totalCount)
	}

	return out, nil
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}
