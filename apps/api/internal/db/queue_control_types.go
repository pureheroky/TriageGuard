package db

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	RequestStateOpen   = "OPEN"
	RequestStateClosed = "CLOSED"

	WaitingOnNone           = "none"
	WaitingOnRequester      = "requester"
	WaitingOnOtherTeam      = "other_team"
	WaitingOnExternalVendor = "external_vendor"
	WaitingOnScheduledWork  = "scheduled_work"

	ClosedReasonResolved   = "resolved"
	ClosedReasonDuplicate  = "duplicate"
	ClosedReasonNotPlanned = "not_planned"
	ClosedReasonNoise      = "noise"
	ClosedReasonInvalid    = "invalid"
)

type Queue struct {
	ID                  uuid.UUID `json:"id"`
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	Name                string    `json:"name"`
	Description         *string   `json:"description,omitempty"`
	DefaultPriority     string    `json:"default_priority"`
	DefaultRequestType  string    `json:"default_request_type"`
	EscalationChannelID *string   `json:"escalation_channel_id,omitempty"`
	IsDefault           bool      `json:"is_default"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type QueuePolicy struct {
	QueueID              uuid.UUID `json:"queue_id"`
	AckSLAMinutes        int       `json:"ack_sla_minutes"`
	AssignSLAMinutes     int       `json:"assign_sla_minutes"`
	StaleHours           int       `json:"stale_hours"`
	DigestTime           string    `json:"digest_time"`
	DigestWeekday        *int      `json:"digest_weekday,omitempty"`
	AssignStartsFrom     string    `json:"assign_starts_from"`
	StaleStartsFrom      string    `json:"stale_starts_from"`
	Timezone             string    `json:"timezone"`
	BusinessHoursEnabled bool      `json:"business_hours_enabled"`
	BusinessHoursStart   *string   `json:"business_hours_start,omitempty"`
	BusinessHoursEnd     *string   `json:"business_hours_end,omitempty"`
	BusinessDaysMask     int       `json:"business_days_mask"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type QueueMember struct {
	QueueID   uuid.UUID `json:"queue_id"`
	UserID    string    `json:"user_id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type QueueRoutingRule struct {
	ID             uuid.UUID       `json:"id"`
	WorkspaceID    uuid.UUID       `json:"workspace_id"`
	Name           string          `json:"name"`
	SortOrder      int             `json:"sort_order"`
	IsEnabled      bool            `json:"is_enabled"`
	ConditionsJSON json.RawMessage `json:"conditions_json"`
	QueueID        uuid.UUID       `json:"queue_id"`
	RequestType    *string         `json:"request_type,omitempty"`
	Priority       *string         `json:"priority,omitempty"`
	StopProcessing bool            `json:"stop_processing"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type RequestSLAClock struct {
	ID             uuid.UUID  `json:"id"`
	RequestID      uuid.UUID  `json:"request_id"`
	ClockType      string     `json:"clock_type"`
	StartedAt      time.Time  `json:"started_at"`
	TargetAt       *time.Time `json:"target_at,omitempty"`
	PausedAt       *time.Time `json:"paused_at,omitempty"`
	SatisfiedAt    *time.Time `json:"satisfied_at,omitempty"`
	BreachedAt     *time.Time `json:"breached_at,omitempty"`
	LastNotifiedAt *time.Time `json:"last_notified_at,omitempty"`
	State          string     `json:"state"`
	CreatedAt      time.Time  `json:"created_at"`
}

type RequestEvent struct {
	ID         uuid.UUID       `json:"id"`
	RequestID  uuid.UUID       `json:"request_id"`
	EventType  string          `json:"event_type"`
	ActorType  string          `json:"actor_type"`
	ActorID    *string         `json:"actor_id,omitempty"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload_json"`
}

type NotificationOutboxItem struct {
	ID               uuid.UUID       `json:"id"`
	RequestID        *uuid.UUID      `json:"request_id,omitempty"`
	EventID          *uuid.UUID      `json:"event_id,omitempty"`
	Kind             string          `json:"kind"`
	DestinationType  string          `json:"destination_type"`
	DestinationValue *string         `json:"destination_value,omitempty"`
	Payload          json.RawMessage `json:"payload_json"`
	DedupeKey        *string         `json:"dedupe_key,omitempty"`
	Status           string          `json:"status"`
	RetryCount       int             `json:"retry_count"`
	AvailableAt      time.Time       `json:"available_at"`
	SentAt           *time.Time      `json:"sent_at,omitempty"`
	FailedAt         *time.Time      `json:"failed_at,omitempty"`
	LastError        *string         `json:"last_error,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type ExternalIssue struct {
	ID            uuid.UUID  `json:"id"`
	RequestID     uuid.UUID  `json:"request_id"`
	Provider      string     `json:"provider"`
	ExternalID    string     `json:"external_id"`
	ExternalKey   *string    `json:"external_key,omitempty"`
	ExternalURL   *string    `json:"external_url,omitempty"`
	SyncState     string     `json:"sync_state"`
	LastSyncAt    *time.Time `json:"last_sync_at,omitempty"`
	LastSyncError *string    `json:"last_sync_error,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type EscalationStep struct {
	ID                      uuid.UUID `json:"id"`
	QueueID                 uuid.UUID `json:"queue_id"`
	ClockType               string    `json:"clock_type"`
	DelayMinutesAfterBreach int       `json:"delay_minutes_after_breach"`
	TargetType              string    `json:"target_type"`
	TargetValue             *string   `json:"target_value,omitempty"`
	MessageTemplate         *string   `json:"message_template,omitempty"`
	IsEnabled               bool      `json:"is_enabled"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

type DailyQueueMetric struct {
	Date                string    `json:"date"`
	WorkspaceID         uuid.UUID `json:"workspace_id"`
	QueueID             uuid.UUID `json:"queue_id"`
	CreatedCount        int       `json:"created_count"`
	ClosedCount         int       `json:"closed_count"`
	UnackedCount        int       `json:"unacked_count"`
	UnassignedCount     int       `json:"unassigned_count"`
	BreachedAckCount    int       `json:"breached_ack_count"`
	BreachedAssignCount int       `json:"breached_assign_count"`
	MedianAckMinutes    *float64  `json:"median_ack_minutes,omitempty"`
	MedianAssignMinutes *float64  `json:"median_assign_minutes,omitempty"`
	MedianCloseMinutes  *float64  `json:"median_close_minutes,omitempty"`
	ReopenCount         int       `json:"reopen_count"`
}

type WorkspaceEntitlements struct {
	ChannelLimit       *int `json:"channel_limit,omitempty"`
	TriagerLimit       *int `json:"triager_limit,omitempty"`
	SharedPolicyMode   bool `json:"shared_policy_mode"`
	CustomEscalations  bool `json:"custom_escalations"`
	AdvancedAnalytics  bool `json:"advanced_analytics"`
	Exports            bool `json:"exports"`
	MultipleQueueRules bool `json:"multiple_queue_policies"`
}

type QueueSummaryRow struct {
	Queue                Queue       `json:"queue"`
	Policy               QueuePolicy `json:"policy"`
	OpenCount            int         `json:"open_count"`
	UnackedCount         int         `json:"unacked_count"`
	UnassignedCount      int         `json:"unassigned_count"`
	WaitingCount         int         `json:"waiting_count"`
	BreachedCount        int         `json:"breached_count"`
	BreachedStaleCount   int         `json:"breached_stale_count"`
	AtRiskCount          int         `json:"at_risk_count"`
	WaitingDebtCount     int         `json:"waiting_debt_count"`
	NoHumanActivityCount int         `json:"no_human_activity_count"`
	MedianAckMinutes     *float64    `json:"median_ack_minutes,omitempty"`
	MedianAssignMinutes  *float64    `json:"median_assign_minutes,omitempty"`
	MedianCloseMinutes   *float64    `json:"median_close_minutes,omitempty"`
}

type QueueDashboardSummary struct {
	OpenCount            int      `json:"open_count"`
	UnackedCount         int      `json:"unacked_count"`
	UnassignedCount      int      `json:"unassigned_count"`
	WaitingCount         int      `json:"waiting_count"`
	BreachedCount        int      `json:"breached_count"`
	AtRiskCount          int      `json:"at_risk_count"`
	WaitingDebtCount     int      `json:"waiting_debt_count"`
	NoHumanActivityCount int      `json:"no_human_activity_count"`
	MedianAckMinutes     *float64 `json:"median_ack_minutes,omitempty"`
	MedianAssignMinutes  *float64 `json:"median_assign_minutes,omitempty"`
	MedianCloseMinutes   *float64 `json:"median_close_minutes,omitempty"`
}

type QueueDetailMetrics struct {
	Queue                Queue          `json:"queue"`
	Policy               QueuePolicy    `json:"policy"`
	CreatedCount         int            `json:"created_count"`
	UnackedCount         int            `json:"unacked_count"`
	UnassignedCount      int            `json:"unassigned_count"`
	WaitingCount         int            `json:"waiting_count"`
	BreachedCount        int            `json:"breached_count"`
	AtRiskCount          int            `json:"at_risk_count"`
	WaitingDebtCount     int            `json:"waiting_debt_count"`
	NoHumanActivityCount int            `json:"no_human_activity_count"`
	MedianAckMinutes     *float64       `json:"median_ack_minutes,omitempty"`
	MedianAssignMinutes  *float64       `json:"median_assign_minutes,omitempty"`
	MedianCloseMinutes   *float64       `json:"median_close_minutes,omitempty"`
	ReopenRate           float64        `json:"reopen_rate"`
	CloseReasons         map[string]int `json:"close_reasons"`
}

type AgingBucket struct {
	Label  string `json:"label"`
	Count  int    `json:"count"`
	Window string `json:"window"`
}

type QueueAgingRow struct {
	QueueID         uuid.UUID `json:"queue_id"`
	QueueName       string    `json:"queue_name"`
	OpenCount       int       `json:"open_count"`
	AvgOpenAgeHours float64   `json:"avg_open_age_hours"`
	MaxOpenAgeHours float64   `json:"max_open_age_hours"`
}

type TypeSummaryRow struct {
	RequestType          string   `json:"request_type"`
	OpenCount            int      `json:"open_count"`
	WaitingCount         int      `json:"waiting_count"`
	UnassignedCount      int      `json:"unassigned_count"`
	BreachedCount        int      `json:"breached_count"`
	MedianAckMinutes     *float64 `json:"median_ack_minutes,omitempty"`
	MedianAssignMinutes  *float64 `json:"median_assign_minutes,omitempty"`
	MedianCloseMinutes   *float64 `json:"median_close_minutes,omitempty"`
	NoHumanActivityCount int      `json:"no_human_activity_count"`
}

type QueueDashboardAnalytics struct {
	Summary         QueueDashboardSummary `json:"summary"`
	Queues          []QueueSummaryRow     `json:"queues"`
	TopBreached     []QueueSummaryRow     `json:"top_breached_queues"`
	TopStale        []QueueSummaryRow     `json:"top_stale_queues"`
	AgingByQueue    []QueueAgingRow       `json:"aging_by_queue"`
	AgingByType     []TypeSummaryRow      `json:"aging_by_type"`
	WaitingDebt     []QueueSummaryRow     `json:"waiting_debt"`
	UnassignedDebt  []QueueSummaryRow     `json:"unassigned_debt"`
	NoHumanActivity []QueueSummaryRow     `json:"no_human_activity"`
}

type JobRuntimeStatus struct {
	JobName        string     `json:"job_name"`
	LastStartedAt  *time.Time `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time `json:"last_finished_at,omitempty"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastError      *string    `json:"last_error,omitempty"`
	LastDurationMS *int       `json:"last_duration_ms,omitempty"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type WorkspaceOpsSummary struct {
	DeadLetterCount      int                `json:"dead_letter_count"`
	FailedSlackRefreshes int                `json:"failed_slack_refreshes"`
	FailedEscalations    int                `json:"failed_escalations"`
	FailedTrackerSyncs   int                `json:"failed_tracker_syncs"`
	RetryBacklogCount    int                `json:"retry_backlog_count"`
	OldestPendingAgeMins *int               `json:"oldest_pending_age_minutes,omitempty"`
	SlackConnected       bool               `json:"slack_connected"`
	LinearConnected      bool               `json:"linear_connected"`
	SyncLagMinutes       *int               `json:"sync_lag_minutes,omitempty"`
	HealthBanner         string             `json:"health_banner"`
	Runners              []JobRuntimeStatus `json:"runners"`
}

type InternalOpsSummary struct {
	DeadLetterCount      int                `json:"dead_letter_count"`
	FailedOutboxCount    int                `json:"failed_outbox_count"`
	RetryBacklogCount    int                `json:"retry_backlog_count"`
	OldestPendingAgeMins *int               `json:"oldest_pending_age_minutes,omitempty"`
	Runners              []JobRuntimeStatus `json:"runners"`
	WorkspacesAffected   int                `json:"workspaces_affected"`
}

type DemoSeedRun struct {
	ID          uuid.UUID       `json:"id"`
	WorkspaceID uuid.UUID       `json:"workspace_id"`
	Manifest    json.RawMessage `json:"manifest_json"`
	CreatedBy   *uuid.UUID      `json:"created_by,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

type RoutingRuleNode struct {
	Match      string            `json:"match,omitempty"`
	Conditions []RoutingRuleNode `json:"conditions,omitempty"`
	Field      string            `json:"field,omitempty"`
	Operator   string            `json:"operator,omitempty"`
	Value      any               `json:"value,omitempty"`
}

type RequestMutation struct {
	RequestID    uuid.UUID
	ActorType    string
	ActorID      *string
	Acknowledge  *bool
	OwnerUserID  *string
	Priority     *string
	RequestType  *string
	QueueID      *uuid.UUID
	DueAt        **time.Time
	WaitingOn    *string
	SnoozedUntil **time.Time
	CloseReason  *string
	Comment      *string
	LinkTracker  *bool
	Reopen       bool
}

func NormalizeWaitingOn(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case WaitingOnRequester:
		return WaitingOnRequester
	case WaitingOnOtherTeam:
		return WaitingOnOtherTeam
	case WaitingOnExternalVendor:
		return WaitingOnExternalVendor
	case WaitingOnScheduledWork:
		return WaitingOnScheduledWork
	default:
		return WaitingOnNone
	}
}

func NormalizeRequestState(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), RequestStateClosed) {
		return RequestStateClosed
	}
	return RequestStateOpen
}

func DerivedStatus(req Request) string {
	if NormalizeRequestState(req.State) == RequestStateClosed {
		if req.ClosedReason != nil && strings.EqualFold(strings.TrimSpace(*req.ClosedReason), ClosedReasonResolved) {
			return "Resolved"
		}
		return "Closed"
	}
	if NormalizeWaitingOn(req.WaitingOn) != WaitingOnNone {
		switch NormalizeWaitingOn(req.WaitingOn) {
		case WaitingOnRequester:
			return "Waiting on requester"
		case WaitingOnOtherTeam:
			return "Waiting on other team"
		case WaitingOnExternalVendor:
			return "Waiting on vendor"
		case WaitingOnScheduledWork:
			return "Scheduled"
		}
	}
	if req.OwnerUserID != nil && strings.TrimSpace(*req.OwnerUserID) != "" {
		return "Assigned"
	}
	if req.AcknowledgedAt != nil {
		return "Acked"
	}
	return "New"
}

func ApplyRequestCompatibility(req *Request) {
	if req == nil {
		return
	}
	if strings.TrimSpace(req.State) == "" {
		if strings.EqualFold(strings.TrimSpace(req.Status), "RESOLVED") || strings.EqualFold(strings.TrimSpace(req.Status), "IGNORED") {
			req.State = RequestStateClosed
		} else {
			req.State = RequestStateOpen
		}
	}
	if req.AcknowledgedAt == nil {
		req.AcknowledgedAt = req.AckedAt
	}
	if req.AckedAt == nil {
		req.AckedAt = req.AcknowledgedAt
	}
	if req.OwnerUserID == nil {
		req.OwnerUserID = req.OwnerSlackID
	}
	if req.OwnerSlackID == nil {
		req.OwnerSlackID = req.OwnerUserID
	}
	if req.ResolvedAt == nil && req.ClosedAt != nil && req.ClosedReason != nil && strings.EqualFold(strings.TrimSpace(*req.ClosedReason), ClosedReasonResolved) {
		req.ResolvedAt = req.ClosedAt
	}
	if strings.TrimSpace(req.RequestType) == "" {
		req.RequestType = "other"
	}
	if strings.TrimSpace(req.WaitingOn) == "" {
		req.WaitingOn = WaitingOnNone
	}
	if req.LastHumanActivityAt.IsZero() {
		req.LastHumanActivityAt = req.LastActivityAt
		if req.LastHumanActivityAt.IsZero() {
			req.LastHumanActivityAt = req.CreatedAt
		}
	}
	if req.LastStateChangeAt.IsZero() {
		req.LastStateChangeAt = req.CreatedAt
	}
	switch NormalizeRequestState(req.State) {
	case RequestStateClosed:
		if req.ClosedReason != nil && strings.EqualFold(strings.TrimSpace(*req.ClosedReason), ClosedReasonResolved) {
			req.Status = "RESOLVED"
		} else {
			req.Status = "IGNORED"
		}
	default:
		if req.OwnerUserID != nil && strings.TrimSpace(*req.OwnerUserID) != "" {
			req.Status = "ASSIGNED"
		} else if req.AcknowledgedAt != nil {
			req.Status = "ACKED"
		} else {
			req.Status = "NEW"
		}
	}
}

func sameOptionalString(left, right *string) bool {
	leftValue := ""
	rightValue := ""
	if left != nil {
		leftValue = strings.TrimSpace(*left)
	}
	if right != nil {
		rightValue = strings.TrimSpace(*right)
	}
	return leftValue == rightValue
}
