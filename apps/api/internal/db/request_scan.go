package db

import (
	"fmt"

	"github.com/google/uuid"
)

type requestScanner interface {
	Scan(dest ...any) error
}

func requestSelectColumns(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf(`%sid, %sworkspace_id, %ssource_key, %schannel_id, %sthread_ts, %smessage_ts,
		%sauthor_slack_id, %sbody_text, %stitle, %sstatus, %spriority, %sowner_slack_id, %sdue_at,
		%sacked_at, %sassigned_at, %sresolved_at, %slast_activity_at, %sack_overdue_sent_at,
		%sassign_overdue_sent_at, %sstale_sent_at, %screated_at, %striage_message_ts, %sstate,
		%sacknowledged_at, %sacknowledged_by, %sowner_user_id, %srequest_type, %squeue_id,
		%swaiting_on, %ssnoozed_until, %sclosed_at, %sclosed_by, %sclosed_reason,
		%slast_human_activity_at, %slast_state_change_at`,
		prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix,
		prefix, prefix, prefix, prefix, prefix,
		prefix, prefix,
	)
}

func scanRequest(scanner requestScanner, out *Request) error {
	err := scanner.Scan(
		&out.ID,
		&out.WorkspaceID,
		&out.SourceKey,
		&out.ChannelID,
		&out.ThreadTS,
		&out.MessageTS,
		&out.AuthorSlackID,
		&out.BodyText,
		&out.Title,
		&out.Status,
		&out.Priority,
		&out.OwnerSlackID,
		&out.DueAt,
		&out.AckedAt,
		&out.AssignedAt,
		&out.ResolvedAt,
		&out.LastActivityAt,
		&out.AckOverdueSentAt,
		&out.AssignOverdueSentAt,
		&out.StaleSentAt,
		&out.CreatedAt,
		&out.TriageMessageTS,
		&out.State,
		&out.AcknowledgedAt,
		&out.AcknowledgedBy,
		&out.OwnerUserID,
		&out.RequestType,
		&out.QueueID,
		&out.WaitingOn,
		&out.SnoozedUntil,
		&out.ClosedAt,
		&out.ClosedBy,
		&out.ClosedReason,
		&out.LastHumanActivityAt,
		&out.LastStateChangeAt,
	)
	if err != nil {
		return err
	}
	ApplyRequestCompatibility(out)
	return nil
}

func scanRequestWithInserted(scanner requestScanner, out *Request, inserted *bool) error {
	err := scanner.Scan(
		&out.ID,
		&out.WorkspaceID,
		&out.SourceKey,
		&out.ChannelID,
		&out.ThreadTS,
		&out.MessageTS,
		&out.AuthorSlackID,
		&out.BodyText,
		&out.Title,
		&out.Status,
		&out.Priority,
		&out.OwnerSlackID,
		&out.DueAt,
		&out.AckedAt,
		&out.AssignedAt,
		&out.ResolvedAt,
		&out.LastActivityAt,
		&out.AckOverdueSentAt,
		&out.AssignOverdueSentAt,
		&out.StaleSentAt,
		&out.CreatedAt,
		&out.TriageMessageTS,
		&out.State,
		&out.AcknowledgedAt,
		&out.AcknowledgedBy,
		&out.OwnerUserID,
		&out.RequestType,
		&out.QueueID,
		&out.WaitingOn,
		&out.SnoozedUntil,
		&out.ClosedAt,
		&out.ClosedBy,
		&out.ClosedReason,
		&out.LastHumanActivityAt,
		&out.LastStateChangeAt,
		inserted,
	)
	if err != nil {
		return err
	}
	ApplyRequestCompatibility(out)
	return nil
}

func uuidPtr(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	value := id
	return &value
}
