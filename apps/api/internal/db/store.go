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
	"github.com/jackc/pgx/v5/pgxpool"

	"triageguard/apps/api/internal/secret"
)

type Store struct {
	pool        *pgxpool.Pool
	tokenCipher *secret.TokenCipher
}

func NewStore(pool *pgxpool.Pool, tokenCipher *secret.TokenCipher) *Store {
	return &Store{
		pool:        pool,
		tokenCipher: tokenCipher,
	}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *Store) encryptToken(value string) (string, error) {
	if s.tokenCipher == nil {
		return strings.TrimSpace(value), nil
	}
	return s.tokenCipher.Encrypt(strings.TrimSpace(value))
}

func (s *Store) decryptToken(value string) (string, error) {
	if s.tokenCipher == nil {
		return strings.TrimSpace(value), nil
	}
	return s.tokenCipher.Decrypt(strings.TrimSpace(value))
}

type Workspace struct {
	ID   uuid.UUID
	Name string
}

type SlackInstallation struct {
	WorkspaceID uuid.UUID
	TeamID      string
	TeamName    string
	TeamDomain  string
	BotUserID   string
	BotToken    string
}

type SlackChannel struct {
	ChannelID      string     `json:"channel_id"`
	ChannelName    string     `json:"channel_name"`
	IsPrivate      bool       `json:"is_private"`
	Enabled        bool       `json:"enabled"`
	DefaultQueueID *uuid.UUID `json:"default_queue_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ChannelSLAOverride struct {
	WorkspaceID      uuid.UUID `json:"workspace_id"`
	ChannelID        string    `json:"channel_id"`
	AckSLAMinutes    int       `json:"ack_sla_minutes"`
	AssignSLAMinutes int       `json:"assign_sla_minutes"`
	StaleHours       int       `json:"stale_hours"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Policy struct {
	WorkspaceID         uuid.UUID  `json:"workspace_id"`
	AckSLAMinutes       int        `json:"ack_sla_minutes"`
	AssignSLAMinutes    int        `json:"assign_sla_minutes"`
	StaleHours          int        `json:"stale_hours"`
	DailyDigestTime     string     `json:"daily_digest_time"`
	Timezone            string     `json:"timezone"`
	EscalationChannelID *string    `json:"escalation_channel_id"`
	LastDigestSentAt    *time.Time `json:"last_digest_sent_at"`
	LinearTeamID        *string    `json:"linear_team_id"`
}

type WorkspaceSubscription struct {
	WorkspaceID          uuid.UUID  `json:"workspace_id"`
	PlanKey              string     `json:"plan_key"`
	Status               string     `json:"status"`
	BillingProvider      string     `json:"billing_provider"`
	StripeCustomerID     *string    `json:"stripe_customer_id,omitempty"`
	StripeSubscriptionID *string    `json:"stripe_subscription_id,omitempty"`
	StripeCheckoutID     *string    `json:"stripe_checkout_session_id,omitempty"`
	PayPalPayerEmail     *string    `json:"paypal_payer_email,omitempty"`
	PayPalLastPaymentAt  *time.Time `json:"paypal_last_payment_at,omitempty"`
	CancelAtPeriodEnd    bool       `json:"cancel_at_period_end"`
	CurrentPeriodEnd     *time.Time `json:"current_period_end,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

type Request struct {
	ID                  uuid.UUID  `json:"id"`
	WorkspaceID         uuid.UUID  `json:"workspace_id"`
	SourceKey           string     `json:"source_key"`
	ChannelID           string     `json:"channel_id"`
	ThreadTS            string     `json:"thread_ts"`
	MessageTS           string     `json:"message_ts"`
	AuthorSlackID       string     `json:"author_slack_id"`
	BodyText            string     `json:"body_text"`
	Title               *string    `json:"title"`
	Status              string     `json:"status"`
	Priority            string     `json:"priority"`
	OwnerSlackID        *string    `json:"owner_slack_id"`
	DueAt               *time.Time `json:"due_at"`
	AckedAt             *time.Time `json:"acked_at"`
	AssignedAt          *time.Time `json:"assigned_at"`
	ResolvedAt          *time.Time `json:"resolved_at"`
	LastActivityAt      time.Time  `json:"last_activity_at"`
	AckOverdueSentAt    *time.Time
	AssignOverdueSentAt *time.Time
	StaleSentAt         *time.Time
	CreatedAt           time.Time `json:"created_at"`
	TriageMessageTS     *string   `json:"triage_message_ts"`

	State               string     `json:"state"`
	AcknowledgedAt      *time.Time `json:"acknowledged_at"`
	AcknowledgedBy      *string    `json:"acknowledged_by"`
	OwnerUserID         *string    `json:"owner_user_id"`
	RequestType         string     `json:"request_type"`
	QueueID             *uuid.UUID `json:"queue_id"`
	WaitingOn           string     `json:"waiting_on"`
	SnoozedUntil        *time.Time `json:"snoozed_until"`
	ClosedAt            *time.Time `json:"closed_at"`
	ClosedBy            *string    `json:"closed_by"`
	ClosedReason        *string    `json:"closed_reason"`
	LastHumanActivityAt time.Time  `json:"last_human_activity_at"`
	LastStateChangeAt   time.Time  `json:"last_state_change_at"`
}

type LinearInstallation struct {
	WorkspaceID    uuid.UUID
	AccessToken    string
	RefreshToken   *string
	TokenExpiresAt *time.Time
	LastRefreshAt  *time.Time
}

type LinearIssue struct {
	RequestID uuid.UUID `json:"request_id"`
	IssueID   string    `json:"issue_id"`
	IssueURL  string    `json:"issue_url"`
}

type RequestAction struct {
	RequestID    uuid.UUID       `json:"request_id"`
	ActorSlackID string          `json:"actor_slack_id"`
	ActionType   string          `json:"action_type"`
	Payload      json.RawMessage `json:"payload"`
}

type RequestSummary struct {
	NewCount        int `json:"new_count"`
	UnassignedCount int `json:"unassigned_count"`
	OverdueCount    int `json:"overdue_count"`
	AssignedCount   int `json:"assigned_count"`
	OpenCount       int `json:"open_count"`
}

type OverdueRow struct {
	ID           uuid.UUID  `json:"id"`
	Status       string     `json:"status"`
	Priority     string     `json:"priority"`
	Title        *string    `json:"title"`
	ChannelID    string     `json:"channel_id"`
	ThreadTS     string     `json:"thread_ts"`
	ThreadURL    *string    `json:"thread_url,omitempty"`
	OwnerSlackID *string    `json:"owner_slack_id"`
	CreatedAt    time.Time  `json:"created_at"`
	DueAt        *time.Time `json:"due_at"`
}

type RequestDurationMetric struct {
	AvgMinutes float64 `json:"avg_minutes"`
	SampleSize int     `json:"sample_size"`
}

type RequestTrendPoint struct {
	Date              string   `json:"date"`
	AckAvgMinutes     *float64 `json:"ack_avg_minutes,omitempty"`
	AckSampleSize     int      `json:"ack_sample_size"`
	ResolveAvgMinutes *float64 `json:"resolve_avg_minutes,omitempty"`
	ResolveSampleSize int      `json:"resolve_sample_size"`
}

type RequestAnalytics struct {
	WindowDays int                   `json:"window_days"`
	Ack        RequestDurationMetric `json:"ack"`
	Resolve    RequestDurationMetric `json:"resolve"`
	Trend      []RequestTrendPoint   `json:"trend"`
}

type RequestReportRow struct {
	ID                  uuid.UUID
	ChannelID           string
	ChannelName         *string
	ThreadTS            string
	ThreadURL           *string
	Title               *string
	Status              string
	Priority            string
	OwnerSlackID        *string
	CreatedAt           time.Time
	AckedAt             *time.Time
	AssignedAt          *time.Time
	ResolvedAt          *time.Time
	DueAt               *time.Time
	AckOverdueSentAt    *time.Time
	AssignOverdueSentAt *time.Time
	StaleSentAt         *time.Time
	LinearIssueID       *string
	LinearIssueURL      *string
}

type ActivityRow struct {
	ID         uuid.UUID       `json:"id"`
	RequestID  uuid.UUID       `json:"request_id"`
	ActionType string          `json:"action_type"`
	ActorType  string          `json:"actor_type"`
	ActorID    *string         `json:"actor_id,omitempty"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

type DeadLetter struct {
	ID           uuid.UUID       `json:"id"`
	Source       string          `json:"source"`
	Operation    string          `json:"operation"`
	WorkspaceID  *uuid.UUID      `json:"workspace_id,omitempty"`
	RequestID    *uuid.UUID      `json:"request_id,omitempty"`
	ExternalID   *string         `json:"external_id,omitempty"`
	ErrorMessage string          `json:"error_message"`
	Payload      json.RawMessage `json:"payload"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (s *Store) GetAnyWorkspace(ctx context.Context) (Workspace, error) {
	var w Workspace
	err := s.pool.QueryRow(ctx, `
		select w.id, w.name
		from workspaces w
		left join slack_installations si on si.workspace_id = w.id
		order by
			case when si.workspace_id is null then 1 else 0 end asc,
			coalesce(si.installed_at, w.created_at) desc,
			w.created_at desc
		limit 1
	`).Scan(&w.ID, &w.Name)
	if err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func (s *Store) GetWorkspaceForUser(ctx context.Context, userID uuid.UUID, preferredWorkspaceID *uuid.UUID) (Workspace, error) {
	if preferredWorkspaceID != nil && *preferredWorkspaceID != uuid.Nil {
		var preferred Workspace
		err := s.pool.QueryRow(ctx, `
			select w.id, w.name
			from workspaces w
			join workspace_members wm on wm.workspace_id = w.id
			where wm.user_id = $1 and w.id = $2
		`, userID, *preferredWorkspaceID).Scan(&preferred.ID, &preferred.Name)
		if err == nil {
			return preferred, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Workspace{}, err
		}
	}

	var w Workspace
	err := s.pool.QueryRow(ctx, `
		select w.id, w.name
		from workspaces w
		join workspace_members wm on wm.workspace_id = w.id
		left join slack_installations si on si.workspace_id = w.id
		where wm.user_id = $1
		order by
			case when wm.role = 'owner' then 0 else 1 end asc,
			case when si.workspace_id is null then 1 else 0 end asc,
			wm.created_at asc,
			w.created_at asc
		limit 1
	`, userID).Scan(&w.ID, &w.Name)
	if err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func (s *Store) EnsureWorkspaceMember(ctx context.Context, workspaceID, userID uuid.UUID, role string) error {
	if role != "owner" && role != "admin" {
		role = "admin"
	}
	_, err := s.pool.Exec(ctx, `
		insert into workspace_members (workspace_id, user_id, role)
		values ($1, $2, $3)
		on conflict (workspace_id, user_id) do nothing
	`, workspaceID, userID, role)
	return err
}

func (s *Store) ProvisionWorkspaceForUser(ctx context.Context, userID uuid.UUID) (Workspace, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `select pg_advisory_xact_lock(hashtext($1)::bigint)`, userID.String()); err != nil {
		return Workspace{}, err
	}

	var existing Workspace
	err = tx.QueryRow(ctx, `
		select w.id, w.name
		from workspaces w
		join workspace_members wm on wm.workspace_id = w.id
		left join slack_installations si on si.workspace_id = w.id
		where wm.user_id = $1
		order by
			case when wm.role = 'owner' then 0 else 1 end asc,
			case when si.workspace_id is null then 1 else 0 end asc,
			wm.created_at asc,
			w.created_at asc
		limit 1
	`, userID).Scan(&existing.ID, &existing.Name)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Workspace{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Workspace{}, err
	}

	workspaceName := fmt.Sprintf("Workspace %s", strings.ToUpper(userID.String()[:8]))
	var workspace Workspace
	if err := tx.QueryRow(ctx, `
		insert into workspaces (name)
		values ($1)
		returning id, name
	`, workspaceName).Scan(&workspace.ID, &workspace.Name); err != nil {
		return Workspace{}, err
	}

	if _, err := tx.Exec(ctx, `
		insert into workspace_members (workspace_id, user_id, role)
		values ($1, $2, 'owner')
		on conflict (workspace_id, user_id) do nothing
	`, workspace.ID, userID); err != nil {
		return Workspace{}, err
	}

	if _, err := tx.Exec(ctx, `
		insert into policies (workspace_id)
		values ($1)
		on conflict (workspace_id) do nothing
	`, workspace.ID); err != nil {
		return Workspace{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (s *Store) BootstrapSingleWorkspaceMembership(ctx context.Context, userID uuid.UUID) (Workspace, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx)

	var memberCount int
	if err := tx.QueryRow(ctx, `select count(*) from workspace_members`).Scan(&memberCount); err != nil {
		return Workspace{}, err
	}
	if memberCount != 0 {
		return Workspace{}, pgx.ErrNoRows
	}

	var workspaceCount int
	if err := tx.QueryRow(ctx, `select count(*) from workspaces`).Scan(&workspaceCount); err != nil {
		return Workspace{}, err
	}
	if workspaceCount != 1 {
		return Workspace{}, pgx.ErrNoRows
	}

	var workspace Workspace
	if err := tx.QueryRow(ctx, `
		select id, name
		from workspaces
		order by created_at desc
		limit 1
	`).Scan(&workspace.ID, &workspace.Name); err != nil {
		return Workspace{}, err
	}

	if _, err := tx.Exec(ctx, `
		insert into workspace_members (workspace_id, user_id, role)
		values ($1, $2, 'owner')
		on conflict (workspace_id, user_id) do nothing
	`, workspace.ID, userID); err != nil {
		return Workspace{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (s *Store) GetWorkspaceByTeamID(ctx context.Context, teamID string) (Workspace, error) {
	var w Workspace
	err := s.pool.QueryRow(ctx, `
		select w.id, w.name
		from workspaces w
		join slack_installations si on si.workspace_id = w.id
		where si.team_id = $1
	`, teamID).Scan(&w.ID, &w.Name)
	if err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func (s *Store) UpsertSlackInstallation(ctx context.Context, teamID, teamName, teamDomain, botUserID, botToken string) (Workspace, error) {
	encryptedBotToken, err := s.encryptToken(botToken)
	if err != nil {
		return Workspace{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx)

	workspaceName := teamName
	if workspaceName == "" {
		workspaceName = teamID
	}
	var workspaceID uuid.UUID
	err = tx.QueryRow(ctx, `
		select workspace_id
		from slack_installations
		where team_id = $1
	`, teamID).Scan(&workspaceID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return Workspace{}, err
		}
		err = tx.QueryRow(ctx, `
			insert into workspaces (name)
			values ($1)
			returning id
		`, workspaceName).Scan(&workspaceID)
		if err != nil {
			return Workspace{}, err
		}
	}

	_, err = tx.Exec(ctx, `
		insert into slack_installations (workspace_id, team_id, team_name, team_domain, bot_user_id, bot_token)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (team_id) do update
		set team_name = excluded.team_name,
			team_domain = excluded.team_domain,
			bot_user_id = excluded.bot_user_id,
			bot_token = excluded.bot_token,
			installed_at = now()
	`, workspaceID, teamID, teamName, teamDomain, botUserID, encryptedBotToken)
	if err != nil {
		return Workspace{}, err
	}

	_, err = tx.Exec(ctx, `
		insert into policies (workspace_id)
		values ($1)
		on conflict (workspace_id) do nothing
	`, workspaceID)
	if err != nil {
		return Workspace{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, err
	}
	return Workspace{ID: workspaceID, Name: workspaceName}, nil
}

func (s *Store) UpsertSlackInstallationForWorkspace(ctx context.Context, workspaceID uuid.UUID, teamID, teamName, teamDomain, botUserID, botToken string) (Workspace, error) {
	encryptedBotToken, err := s.encryptToken(botToken)
	if err != nil {
		return Workspace{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Workspace{}, err
	}
	defer tx.Rollback(ctx)

	var workspace Workspace
	if err := tx.QueryRow(ctx, `
		select id, name
		from workspaces
		where id = $1
	`, workspaceID).Scan(&workspace.ID, &workspace.Name); err != nil {
		return Workspace{}, err
	}

	if _, err := tx.Exec(ctx, `
		delete from slack_installations
		where team_id = $1 and workspace_id <> $2
	`, teamID, workspaceID); err != nil {
		return Workspace{}, err
	}

	_, err = tx.Exec(ctx, `
		insert into slack_installations (workspace_id, team_id, team_name, team_domain, bot_user_id, bot_token)
		values ($1, $2, $3, $4, $5, $6)
		on conflict (workspace_id) do update
		set team_id = excluded.team_id,
			team_name = excluded.team_name,
			team_domain = excluded.team_domain,
			bot_user_id = excluded.bot_user_id,
			bot_token = excluded.bot_token,
			installed_at = now()
	`, workspaceID, teamID, teamName, teamDomain, botUserID, encryptedBotToken)
	if err != nil {
		return Workspace{}, err
	}

	if _, err := tx.Exec(ctx, `
		insert into policies (workspace_id)
		values ($1)
		on conflict (workspace_id) do nothing
	`, workspaceID); err != nil {
		return Workspace{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return Workspace{}, err
	}
	return workspace, nil
}

func (s *Store) GetSlackInstallationByWorkspace(ctx context.Context, workspaceID uuid.UUID) (SlackInstallation, error) {
	var i SlackInstallation
	err := s.pool.QueryRow(ctx, `
		select workspace_id, team_id, coalesce(team_name, ''), coalesce(team_domain, ''), bot_user_id, bot_token
		from slack_installations
		where workspace_id = $1
	`, workspaceID).Scan(&i.WorkspaceID, &i.TeamID, &i.TeamName, &i.TeamDomain, &i.BotUserID, &i.BotToken)
	if err != nil {
		return SlackInstallation{}, err
	}
	i.BotToken, err = s.decryptToken(i.BotToken)
	if err != nil {
		return SlackInstallation{}, err
	}
	return i, nil
}

func (s *Store) GetSlackInstallationByTeam(ctx context.Context, teamID string) (SlackInstallation, error) {
	var i SlackInstallation
	err := s.pool.QueryRow(ctx, `
		select workspace_id, team_id, coalesce(team_name, ''), coalesce(team_domain, ''), bot_user_id, bot_token
		from slack_installations
		where team_id = $1
	`, teamID).Scan(&i.WorkspaceID, &i.TeamID, &i.TeamName, &i.TeamDomain, &i.BotUserID, &i.BotToken)
	if err != nil {
		return SlackInstallation{}, err
	}
	i.BotToken, err = s.decryptToken(i.BotToken)
	if err != nil {
		return SlackInstallation{}, err
	}
	return i, nil
}

func (s *Store) DisconnectSlack(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		delete from slack_installations
		where workspace_id = $1
	`, workspaceID)
	if err != nil {
		return false, err
	}
	disconnected := tag.RowsAffected() > 0

	if _, err := tx.Exec(ctx, `
		delete from slack_channels
		where workspace_id = $1
	`, workspaceID); err != nil {
		return false, err
	}

	if _, err := tx.Exec(ctx, `
		update policies
		set escalation_channel_id = null
		where workspace_id = $1
	`, workspaceID); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return disconnected, nil
}

func (s *Store) DeleteWorkspace(ctx context.Context, workspaceID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		delete from workspaces
		where id = $1
	`, workspaceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) ListChannels(ctx context.Context, workspaceID uuid.UUID) ([]SlackChannel, error) {
	rows, err := s.pool.Query(ctx, `
		select channel_id, channel_name, is_private, enabled, default_queue_id, created_at
		from slack_channels
		where workspace_id = $1
		order by channel_name asc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]SlackChannel, 0)
	for rows.Next() {
		var ch SlackChannel
		if err := rows.Scan(&ch.ChannelID, &ch.ChannelName, &ch.IsPrivate, &ch.Enabled, &ch.DefaultQueueID, &ch.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *Store) UpsertChannels(ctx context.Context, workspaceID uuid.UUID, channels []SlackChannel) error {
	if len(channels) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, ch := range channels {
		batch.Queue(`
			insert into slack_channels (workspace_id, channel_id, channel_name, is_private, enabled, default_queue_id)
			values ($1, $2, $3, $4, coalesce((select enabled from slack_channels where workspace_id = $1 and channel_id = $2), false), (select default_queue_id from slack_channels where workspace_id = $1 and channel_id = $2))
			on conflict (workspace_id, channel_id) do update
			set channel_name = excluded.channel_name,
				is_private = excluded.is_private
		`, workspaceID, ch.ChannelID, ch.ChannelName, ch.IsPrivate)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range channels {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateChannelStates(ctx context.Context, workspaceID uuid.UUID, updates map[string]bool) error {
	if len(updates) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for channelID, enabled := range updates {
		batch.Queue(`
			update slack_channels
			set enabled = $3
			where workspace_id = $1 and channel_id = $2
		`, workspaceID, channelID, enabled)
	}
	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range updates {
		if _, err := br.Exec(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetChannelSLAOverride(ctx context.Context, workspaceID uuid.UUID, channelID string) (ChannelSLAOverride, error) {
	var out ChannelSLAOverride
	err := s.pool.QueryRow(ctx, `
		select workspace_id, channel_id, ack_sla_minutes, assign_sla_minutes, stale_hours, created_at, updated_at
		from channel_sla_overrides
		where workspace_id = $1 and channel_id = $2
	`, workspaceID, strings.TrimSpace(channelID)).Scan(
		&out.WorkspaceID,
		&out.ChannelID,
		&out.AckSLAMinutes,
		&out.AssignSLAMinutes,
		&out.StaleHours,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return ChannelSLAOverride{}, err
	}
	return out, nil
}

func (s *Store) ListChannelSLAOverrides(ctx context.Context, workspaceID uuid.UUID) ([]ChannelSLAOverride, error) {
	rows, err := s.pool.Query(ctx, `
		select workspace_id, channel_id, ack_sla_minutes, assign_sla_minutes, stale_hours, created_at, updated_at
		from channel_sla_overrides
		where workspace_id = $1
		order by channel_id asc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ChannelSLAOverride, 0)
	for rows.Next() {
		var item ChannelSLAOverride
		if err := rows.Scan(
			&item.WorkspaceID,
			&item.ChannelID,
			&item.AckSLAMinutes,
			&item.AssignSLAMinutes,
			&item.StaleHours,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ReplaceChannelSLAOverrides(ctx context.Context, workspaceID uuid.UUID, overrides []ChannelSLAOverride) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		delete from channel_sla_overrides
		where workspace_id = $1
	`, workspaceID); err != nil {
		return err
	}

	for _, item := range overrides {
		if _, err := tx.Exec(ctx, `
			insert into channel_sla_overrides (
				workspace_id, channel_id, ack_sla_minutes, assign_sla_minutes, stale_hours, created_at, updated_at
			) values ($1, $2, $3, $4, $5, now(), now())
		`, workspaceID, strings.TrimSpace(item.ChannelID), item.AckSLAMinutes, item.AssignSLAMinutes, item.StaleHours); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (s *Store) IsChannelEnabled(ctx context.Context, workspaceID uuid.UUID, channelID string) (bool, error) {
	var enabled bool
	err := s.pool.QueryRow(ctx, `
		select enabled from slack_channels where workspace_id = $1 and channel_id = $2
	`, workspaceID, channelID).Scan(&enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return enabled, nil
}

func (s *Store) GetPolicy(ctx context.Context, workspaceID uuid.UUID) (Policy, error) {
	var p Policy
	err := s.pool.QueryRow(ctx, `
		select workspace_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
		to_char(daily_digest_time, 'HH24:MI:SS'), timezone, escalation_channel_id,
		last_digest_sent_at, linear_team_id
		from policies
		where workspace_id = $1
	`, workspaceID).Scan(
		&p.WorkspaceID,
		&p.AckSLAMinutes,
		&p.AssignSLAMinutes,
		&p.StaleHours,
		&p.DailyDigestTime,
		&p.Timezone,
		&p.EscalationChannelID,
		&p.LastDigestSentAt,
		&p.LinearTeamID,
	)
	if err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (s *Store) UpdatePolicy(ctx context.Context, p Policy) error {
	_, err := s.pool.Exec(ctx, `
		update policies
		set ack_sla_minutes = $2,
			assign_sla_minutes = $3,
			stale_hours = $4,
			daily_digest_time = $5,
			timezone = $6,
			escalation_channel_id = $7,
			linear_team_id = $8
		where workspace_id = $1
	`, p.WorkspaceID, p.AckSLAMinutes, p.AssignSLAMinutes, p.StaleHours, p.DailyDigestTime, p.Timezone, p.EscalationChannelID, p.LinearTeamID)
	return err
}

func (s *Store) GetWorkspaceSubscription(ctx context.Context, workspaceID uuid.UUID) (WorkspaceSubscription, error) {
	var sub WorkspaceSubscription
	err := s.pool.QueryRow(ctx, `
		select workspace_id, plan_key, status, billing_provider, stripe_customer_id, stripe_subscription_id, stripe_checkout_session_id,
			paypal_payer_email, paypal_last_payment_at, cancel_at_period_end, current_period_end, created_at, updated_at
		from workspace_subscriptions
		where workspace_id = $1
	`, workspaceID).Scan(
		&sub.WorkspaceID,
		&sub.PlanKey,
		&sub.Status,
		&sub.BillingProvider,
		&sub.StripeCustomerID,
		&sub.StripeSubscriptionID,
		&sub.StripeCheckoutID,
		&sub.PayPalPayerEmail,
		&sub.PayPalLastPaymentAt,
		&sub.CancelAtPeriodEnd,
		&sub.CurrentPeriodEnd,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return WorkspaceSubscription{}, err
	}
	return sub, nil
}

func (s *Store) GetWorkspaceSubscriptionOrDefault(ctx context.Context, workspaceID uuid.UUID) (WorkspaceSubscription, error) {
	sub, err := s.GetWorkspaceSubscription(ctx, workspaceID)
	if err == nil {
		return sub, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return WorkspaceSubscription{}, err
	}
	now := time.Now().UTC()
	return WorkspaceSubscription{
		WorkspaceID:       workspaceID,
		PlanKey:           "team",
		Status:            "inactive",
		BillingProvider:   "stripe",
		CancelAtPeriodEnd: false,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

func (s *Store) UpsertWorkspaceSubscription(ctx context.Context, sub WorkspaceSubscription) error {
	plan := strings.ToLower(strings.TrimSpace(sub.PlanKey))
	if plan == "pro" {
		plan = "enterprise"
	}
	if plan == "starter" {
		plan = "team"
	}
	if plan != "team" && plan != "enterprise" {
		plan = "team"
	}
	status := strings.ToLower(strings.TrimSpace(sub.Status))
	if status == "" {
		status = "inactive"
	}
	provider := strings.ToLower(strings.TrimSpace(sub.BillingProvider))
	if provider != "paypal" && provider != "stripe" {
		provider = "stripe"
	}

	_, err := s.pool.Exec(ctx, `
		insert into workspace_subscriptions (
			workspace_id, plan_key, status, billing_provider, stripe_customer_id, stripe_subscription_id, stripe_checkout_session_id,
			paypal_payer_email, paypal_last_payment_at, cancel_at_period_end, current_period_end, created_at, updated_at
		)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now(), now())
		on conflict (workspace_id)
		do update set
			plan_key = excluded.plan_key,
			status = excluded.status,
			billing_provider = excluded.billing_provider,
			stripe_customer_id = coalesce(excluded.stripe_customer_id, workspace_subscriptions.stripe_customer_id),
			stripe_subscription_id = coalesce(excluded.stripe_subscription_id, workspace_subscriptions.stripe_subscription_id),
			stripe_checkout_session_id = coalesce(excluded.stripe_checkout_session_id, workspace_subscriptions.stripe_checkout_session_id),
			paypal_payer_email = coalesce(excluded.paypal_payer_email, workspace_subscriptions.paypal_payer_email),
			paypal_last_payment_at = coalesce(excluded.paypal_last_payment_at, workspace_subscriptions.paypal_last_payment_at),
			cancel_at_period_end = excluded.cancel_at_period_end,
			current_period_end = excluded.current_period_end,
			updated_at = now()
	`, sub.WorkspaceID, plan, status, provider, sub.StripeCustomerID, sub.StripeSubscriptionID, sub.StripeCheckoutID, sub.PayPalPayerEmail, sub.PayPalLastPaymentAt, sub.CancelAtPeriodEnd, sub.CurrentPeriodEnd)
	return err
}

func (s *Store) FindWorkspaceSubscriptionByStripeSubscriptionID(ctx context.Context, stripeSubscriptionID string) (WorkspaceSubscription, error) {
	var sub WorkspaceSubscription
	err := s.pool.QueryRow(ctx, `
		select workspace_id, plan_key, status, billing_provider, stripe_customer_id, stripe_subscription_id, stripe_checkout_session_id,
			paypal_payer_email, paypal_last_payment_at, cancel_at_period_end, current_period_end, created_at, updated_at
		from workspace_subscriptions
		where stripe_subscription_id = $1
	`, strings.TrimSpace(stripeSubscriptionID)).Scan(
		&sub.WorkspaceID,
		&sub.PlanKey,
		&sub.Status,
		&sub.BillingProvider,
		&sub.StripeCustomerID,
		&sub.StripeSubscriptionID,
		&sub.StripeCheckoutID,
		&sub.PayPalPayerEmail,
		&sub.PayPalLastPaymentAt,
		&sub.CancelAtPeriodEnd,
		&sub.CurrentPeriodEnd,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return WorkspaceSubscription{}, err
	}
	return sub, nil
}

func (s *Store) FindWorkspaceSubscriptionByStripeCustomerID(ctx context.Context, stripeCustomerID string) (WorkspaceSubscription, error) {
	var sub WorkspaceSubscription
	err := s.pool.QueryRow(ctx, `
		select workspace_id, plan_key, status, billing_provider, stripe_customer_id, stripe_subscription_id, stripe_checkout_session_id,
			paypal_payer_email, paypal_last_payment_at, cancel_at_period_end, current_period_end, created_at, updated_at
		from workspace_subscriptions
		where stripe_customer_id = $1
	`, strings.TrimSpace(stripeCustomerID)).Scan(
		&sub.WorkspaceID,
		&sub.PlanKey,
		&sub.Status,
		&sub.BillingProvider,
		&sub.StripeCustomerID,
		&sub.StripeSubscriptionID,
		&sub.StripeCheckoutID,
		&sub.PayPalPayerEmail,
		&sub.PayPalLastPaymentAt,
		&sub.CancelAtPeriodEnd,
		&sub.CurrentPeriodEnd,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	)
	if err != nil {
		return WorkspaceSubscription{}, err
	}
	return sub, nil
}

func (s *Store) UpsertRequest(ctx context.Context, req Request) (Request, error) {
	now := time.Now().UTC()
	queue, requestType, priority, err := s.ResolveIntakeRouting(ctx, req.WorkspaceID, req.ChannelID, req.AuthorSlackID, req.BodyText)
	if err != nil {
		return Request{}, err
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Request{}, err
	}
	defer tx.Rollback(ctx)

	var out Request
	var inserted bool
	row := tx.QueryRow(ctx, fmt.Sprintf(`
		insert into requests (
			workspace_id, source_key, channel_id, thread_ts, message_ts,
			author_slack_id, body_text, title, status, priority, last_activity_at,
			state, acknowledged_at, acknowledged_by, owner_user_id, request_type, queue_id,
			waiting_on, snoozed_until, closed_at, closed_by, closed_reason,
			last_human_activity_at, last_state_change_at
		)
		values (
			$1, $2, $3, $4, $5, $6, $7, $8, 'NEW', $9, $10,
			'OPEN', null, null, null, $11, $12, 'none', null, null, null, null, $10, $10
		)
		on conflict (source_key)
		do update set
			body_text = excluded.body_text,
			title = excluded.title,
			message_ts = excluded.message_ts,
			last_activity_at = excluded.last_activity_at,
			last_human_activity_at = excluded.last_human_activity_at,
			priority = coalesce(requests.priority, excluded.priority),
			request_type = coalesce(nullif(requests.request_type, ''), excluded.request_type),
			queue_id = coalesce(requests.queue_id, excluded.queue_id)
		returning %s, (xmax = 0) as inserted
	`, requestSelectColumns("")), req.WorkspaceID, req.SourceKey, req.ChannelID, req.ThreadTS, req.MessageTS, req.AuthorSlackID, req.BodyText, req.Title, priority, now, requestType, queue.ID)
	if err := scanRequestWithInserted(row, &out, &inserted); err != nil {
		return Request{}, err
	}

	if inserted {
		policy, err := getQueuePolicyForRequestTx(ctx, tx, out)
		if err != nil {
			return Request{}, err
		}
		if err := createClockPhaseTx(ctx, tx, out, policy, out.CreatedAt); err != nil {
			return Request{}, err
		}
		eventPayload, _ := json.Marshal(map[string]any{
			"channel_id":     out.ChannelID,
			"thread_ts":      out.ThreadTS,
			"queue_id":       out.QueueID,
			"request_type":   out.RequestType,
			"priority":       out.Priority,
			"derived_status": DerivedStatus(out),
		})
		if _, err := insertRequestEventTx(ctx, tx, RequestEvent{
			RequestID:  out.ID,
			EventType:  "request_created",
			ActorType:  "slack_user",
			ActorID:    &out.AuthorSlackID,
			OccurredAt: out.CreatedAt,
			Payload:    eventPayload,
		}); err != nil {
			return Request{}, err
		}
		if err := enqueueRefreshTx(ctx, tx, out.ID); err != nil {
			return Request{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Store) GetRequestByID(ctx context.Context, requestID uuid.UUID) (Request, error) {
	var out Request
	err := scanRequest(s.pool.QueryRow(ctx, fmt.Sprintf(`
		select %s
		from requests
		where id = $1
	`, requestSelectColumns("")), requestID), &out)
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Store) SaveTriageMessageTS(ctx context.Context, requestID uuid.UUID, ts string) error {
	_, err := s.pool.Exec(ctx, `
		update requests
		set triage_message_ts = $2,
			last_activity_at = now(),
			stale_sent_at = null
		where id = $1
	`, requestID, ts)
	return err
}

func (s *Store) TouchRequestBySourceKey(ctx context.Context, sourceKey string) error {
	_, err := s.pool.Exec(ctx, `
		update requests
		set last_activity_at = now(),
			stale_sent_at = null
		where source_key = $1
	`, sourceKey)
	return err
}

func (s *Store) SetAck(ctx context.Context, requestID uuid.UUID) error {
	ack := true
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID:   requestID,
		ActorType:   "system",
		Acknowledge: &ack,
	})
	return err
}

func (s *Store) SetAssign(ctx context.Context, requestID uuid.UUID, ownerSlackID string) error {
	owner := strings.TrimSpace(ownerSlackID)
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID:   requestID,
		ActorType:   "system",
		OwnerUserID: &owner,
	})
	return err
}

func (s *Store) SetPriority(ctx context.Context, requestID uuid.UUID, priority string) error {
	priority = normalizedPriority(priority)
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID: requestID,
		ActorType: "system",
		Priority:  &priority,
	})
	return err
}

func (s *Store) SetDue(ctx context.Context, requestID uuid.UUID, dueAt *time.Time) error {
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID: requestID,
		ActorType: "system",
		DueAt:     &dueAt,
	})
	return err
}

func (s *Store) SetIgnore(ctx context.Context, requestID uuid.UUID) error {
	reason := ClosedReasonNoise
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID:   requestID,
		ActorType:   "system",
		CloseReason: &reason,
	})
	return err
}

func (s *Store) SetResolve(ctx context.Context, requestID uuid.UUID) error {
	reason := ClosedReasonResolved
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID:   requestID,
		ActorType:   "system",
		CloseReason: &reason,
	})
	return err
}

func (s *Store) SetReopen(ctx context.Context, requestID uuid.UUID) error {
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID: requestID,
		ActorType: "system",
		Reopen:    true,
	})
	return err
}

func (s *Store) InsertAction(ctx context.Context, a RequestAction) error {
	if len(a.Payload) == 0 {
		a.Payload = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		insert into request_actions (request_id, actor_slack_id, action_type, payload)
		values ($1, $2, $3, $4::jsonb)
	`, a.RequestID, a.ActorSlackID, a.ActionType, a.Payload)
	return err
}

func (s *Store) InsertDeadLetter(ctx context.Context, item DeadLetter) error {
	if len(item.Payload) == 0 {
		item.Payload = []byte(`{}`)
	}
	_, err := s.pool.Exec(ctx, `
		insert into dead_letters (
			source, operation, workspace_id, request_id, external_id, error_message, payload
		) values ($1, $2, $3, $4, $5, $6, $7::jsonb)
	`, strings.TrimSpace(item.Source), strings.TrimSpace(item.Operation), item.WorkspaceID, item.RequestID, item.ExternalID, strings.TrimSpace(item.ErrorMessage), item.Payload)
	return err
}

func (s *Store) ListDeadLetters(ctx context.Context, workspaceID uuid.UUID, limit int) ([]DeadLetter, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.pool.Query(ctx, `
		select id, source, operation, workspace_id, request_id, external_id, error_message, payload, created_at
		from dead_letters
		where workspace_id = $1
		order by created_at desc
		limit $2
	`, workspaceID, limit)
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

func (s *Store) TryRecordSlackActionDedup(ctx context.Context, key string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		insert into slack_action_dedup (id)
		values ($1)
		on conflict do nothing
	`, key)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) TryRecordStripeWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error) {
	id := strings.TrimSpace(eventID)
	if id == "" {
		return false, errors.New("stripe event id is required")
	}
	tag, err := s.pool.Exec(ctx, `
		insert into stripe_webhook_events (event_id, event_type)
		values ($1, $2)
		on conflict do nothing
	`, id, strings.TrimSpace(eventType))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) TryRecordLinearWebhookEvent(ctx context.Context, eventID, eventType string) (bool, error) {
	id := strings.TrimSpace(eventID)
	if id == "" {
		return false, errors.New("linear event id is required")
	}
	tag, err := s.pool.Exec(ctx, `
		insert into linear_webhook_events (event_id, event_type)
		values ($1, $2)
		on conflict do nothing
	`, id, strings.TrimSpace(eventType))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) DeleteTerminalRequestsBefore(ctx context.Context, cutoff time.Time, limit int) (int64, error) {
	if limit < 1 {
		limit = 500
	}
	tag, err := s.pool.Exec(ctx, `
		with candidate as (
			select id
			from requests
			where status in ('RESOLVED', 'IGNORED')
				and coalesce(resolved_at, last_activity_at, created_at) < $1
			order by coalesce(resolved_at, last_activity_at, created_at) asc
			limit $2
		)
		delete from requests
		where id in (select id from candidate)
	`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) DeleteSlackActionDedupBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		delete from slack_action_dedup
		where created_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) DeleteStripeWebhookEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		delete from stripe_webhook_events
		where received_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) DeleteLinearWebhookEventsBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	tag, err := s.pool.Exec(ctx, `
		delete from linear_webhook_events
		where received_at < $1
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *Store) TryAdvisoryLock(ctx context.Context, key int64) (bool, error) {
	var locked bool
	if err := s.pool.QueryRow(ctx, `select pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
		return false, err
	}
	return locked, nil
}

func (s *Store) AdvisoryUnlock(ctx context.Context, key int64) error {
	var unlocked bool
	if err := s.pool.QueryRow(ctx, `select pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil {
		return err
	}
	return nil
}

func (s *Store) GetLinearInstallation(ctx context.Context, workspaceID uuid.UUID) (LinearInstallation, error) {
	var out LinearInstallation
	err := s.pool.QueryRow(ctx, `
		select workspace_id, access_token, refresh_token, token_expires_at, last_refreshed_at
		from linear_installations
		where workspace_id = $1
	`, workspaceID).Scan(&out.WorkspaceID, &out.AccessToken, &out.RefreshToken, &out.TokenExpiresAt, &out.LastRefreshAt)
	if err != nil {
		return LinearInstallation{}, err
	}
	out.AccessToken, err = s.decryptToken(out.AccessToken)
	if err != nil {
		return LinearInstallation{}, err
	}
	if out.RefreshToken != nil && strings.TrimSpace(*out.RefreshToken) != "" {
		decryptedRefreshToken, err := s.decryptToken(*out.RefreshToken)
		if err != nil {
			return LinearInstallation{}, err
		}
		out.RefreshToken = &decryptedRefreshToken
	}
	return out, nil
}

func (s *Store) UpsertLinearInstallation(ctx context.Context, workspaceID uuid.UUID, accessToken string, refreshToken *string, tokenExpiresAt *time.Time) error {
	encryptedAccessToken, err := s.encryptToken(accessToken)
	if err != nil {
		return err
	}

	var encryptedRefreshToken *string
	if refreshToken != nil {
		candidate := strings.TrimSpace(*refreshToken)
		if candidate != "" {
			encrypted, err := s.encryptToken(candidate)
			if err != nil {
				return err
			}
			encryptedRefreshToken = &encrypted
		}
	}

	_, err = s.pool.Exec(ctx, `
		insert into linear_installations (workspace_id, access_token, refresh_token, token_expires_at, last_refreshed_at)
		values ($1, $2, $3, $4, now())
		on conflict (workspace_id)
		do update set
			access_token = excluded.access_token,
			refresh_token = coalesce(excluded.refresh_token, linear_installations.refresh_token),
			token_expires_at = excluded.token_expires_at,
			last_refreshed_at = now()
	`, workspaceID, encryptedAccessToken, encryptedRefreshToken, tokenExpiresAt)
	return err
}

func (s *Store) DisconnectLinear(ctx context.Context, workspaceID uuid.UUID) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		delete from linear_installations
		where workspace_id = $1
	`, workspaceID)
	if err != nil {
		return false, err
	}
	disconnected := tag.RowsAffected() > 0

	if _, err := tx.Exec(ctx, `
		update policies
		set linear_team_id = null
		where workspace_id = $1
	`, workspaceID); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return disconnected, nil
}

func (s *Store) UpsertLinearIssue(ctx context.Context, issue LinearIssue) error {
	_, err := s.pool.Exec(ctx, `
		insert into linear_issues (request_id, issue_id, issue_url)
		values ($1, $2, $3)
		on conflict (request_id)
		do update set issue_id = excluded.issue_id, issue_url = excluded.issue_url
	`, issue.RequestID, issue.IssueID, issue.IssueURL)
	if err != nil {
		return err
	}
	_, err = s.UpsertExternalIssue(ctx, ExternalIssue{
		RequestID:   issue.RequestID,
		Provider:    "linear",
		ExternalID:  issue.IssueID,
		ExternalKey: &issue.IssueID,
		ExternalURL: &issue.IssueURL,
		SyncState:   "linked",
	})
	return err
}

func (s *Store) GetLinearIssueByRequest(ctx context.Context, requestID uuid.UUID) (LinearIssue, error) {
	if external, err := s.GetExternalIssueByProvider(ctx, requestID, "linear"); err == nil {
		out := LinearIssue{RequestID: requestID, IssueID: external.ExternalID}
		if external.ExternalURL != nil {
			out.IssueURL = *external.ExternalURL
		}
		return out, nil
	}
	var out LinearIssue
	err := s.pool.QueryRow(ctx, `
		select request_id, issue_id, issue_url
		from linear_issues
		where request_id = $1
	`, requestID).Scan(&out.RequestID, &out.IssueID, &out.IssueURL)
	if err != nil {
		return LinearIssue{}, err
	}
	return out, nil
}

func (s *Store) GetRequestByLinearIssueID(ctx context.Context, issueID string) (Request, error) {
	var out Request
	err := scanRequest(s.pool.QueryRow(ctx, fmt.Sprintf(`
		select %s
		from requests r
		join external_issues ei on ei.request_id = r.id and ei.provider = 'linear'
		where ei.external_id = $1
		limit 1
	`, requestSelectColumns("r")), strings.TrimSpace(issueID)), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		err = scanRequest(s.pool.QueryRow(ctx, fmt.Sprintf(`
			select %s
			from requests r
			join linear_issues li on li.request_id = r.id
			where li.issue_id = $1
			limit 1
		`, requestSelectColumns("r")), strings.TrimSpace(issueID)), &out)
	}
	if err != nil {
		return Request{}, err
	}
	return out, nil
}

func (s *Store) UpdateOwnerFromExternal(ctx context.Context, requestID uuid.UUID, ownerSlackID *string) error {
	_, _, err := s.ApplyRequestMutation(ctx, RequestMutation{
		RequestID:   requestID,
		ActorType:   "external_provider",
		OwnerUserID: ownerSlackID,
	})
	return err
}

func (s *Store) RequestsSummary(ctx context.Context, workspaceID uuid.UUID) (RequestSummary, []OverdueRow, error) {
	var out RequestSummary
	err := s.pool.QueryRow(ctx, `
		select
			count(*) filter (where status = 'NEW') as new_count,
			count(*) filter (where owner_slack_id is null and status not in ('RESOLVED','IGNORED')) as unassigned_count,
			count(*) filter (where status = 'ASSIGNED') as assigned_count,
			count(*) filter (where status in ('NEW','ACKED','ASSIGNED')) as open_count,
			count(*) filter (
				where status in ('NEW','ACKED','ASSIGNED')
				and (
					(due_at is not null and due_at < now())
					or (status='NEW' and ack_overdue_sent_at is not null)
					or (owner_slack_id is null and assign_overdue_sent_at is not null)
				)
			) as overdue_count
		from requests
		where workspace_id = $1
	`, workspaceID).Scan(&out.NewCount, &out.UnassignedCount, &out.AssignedCount, &out.OpenCount, &out.OverdueCount)
	if err != nil {
		return RequestSummary{}, nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select
			r.id,
			r.status,
			r.priority,
			r.title,
			r.channel_id,
			r.thread_ts,
			case
				when si.team_domain is null or si.team_domain = '' then null
				else 'https://' || si.team_domain || '.slack.com/archives/' || r.channel_id || '/p' || replace(r.thread_ts, '.', '')
			end as thread_url,
			r.owner_slack_id,
			r.created_at,
			r.due_at
		from requests r
		left join slack_installations si on si.workspace_id = r.workspace_id
		where r.workspace_id = $1
		and status in ('NEW','ACKED','ASSIGNED')
		and (
			(due_at is not null and due_at < now())
			or (status='NEW' and ack_overdue_sent_at is not null)
			or (owner_slack_id is null and assign_overdue_sent_at is not null)
		)
		order by r.created_at asc
		limit 20
	`, workspaceID)
	if err != nil {
		return RequestSummary{}, nil, err
	}
	defer rows.Close()

	list := make([]OverdueRow, 0)
	for rows.Next() {
		var r OverdueRow
		if err := rows.Scan(
			&r.ID,
			&r.Status,
			&r.Priority,
			&r.Title,
			&r.ChannelID,
			&r.ThreadTS,
			&r.ThreadURL,
			&r.OwnerSlackID,
			&r.CreatedAt,
			&r.DueAt,
		); err != nil {
			return RequestSummary{}, nil, err
		}
		list = append(list, r)
	}
	return out, list, rows.Err()
}

func (s *Store) RequestAnalytics(ctx context.Context, workspaceID uuid.UUID, windowDays int, timezone string) (RequestAnalytics, error) {
	if windowDays != 7 && windowDays != 30 && windowDays != 90 {
		windowDays = 30
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "UTC"
	}

	out := RequestAnalytics{
		WindowDays: windowDays,
		Trend:      make([]RequestTrendPoint, 0),
	}

	if err := s.pool.QueryRow(ctx, `
		select
			coalesce((
				select avg(extract(epoch from (acked_at - created_at)) / 60.0)
				from requests
				where workspace_id = $1
				  and acked_at is not null
				  and acked_at >= now() - make_interval(days => $2)
			), 0) as ack_avg_minutes,
			coalesce((
				select count(*)
				from requests
				where workspace_id = $1
				  and acked_at is not null
				  and acked_at >= now() - make_interval(days => $2)
			), 0) as ack_sample_size,
			coalesce((
				select avg(extract(epoch from (resolved_at - created_at)) / 60.0)
				from requests
				where workspace_id = $1
				  and resolved_at is not null
				  and resolved_at >= now() - make_interval(days => $2)
			), 0) as resolve_avg_minutes,
			coalesce((
				select count(*)
				from requests
				where workspace_id = $1
				  and resolved_at is not null
				  and resolved_at >= now() - make_interval(days => $2)
			), 0) as resolve_sample_size
	`, workspaceID, windowDays).Scan(
		&out.Ack.AvgMinutes,
		&out.Ack.SampleSize,
		&out.Resolve.AvgMinutes,
		&out.Resolve.SampleSize,
	); err != nil {
		return RequestAnalytics{}, err
	}

	rows, err := s.pool.Query(ctx, `
		with days as (
			select generate_series(
				((now() at time zone $3)::date - ($2::int - 1)),
				(now() at time zone $3)::date,
				'1 day'::interval
			)::date as day
		),
		ack as (
			select
				(acked_at at time zone $3)::date as day,
				avg(extract(epoch from (acked_at - created_at)) / 60.0) as avg_minutes,
				count(*) as sample_size
			from requests
			where workspace_id = $1
			  and acked_at is not null
			  and acked_at >= now() - make_interval(days => $2)
			group by 1
		),
		resolve as (
			select
				(resolved_at at time zone $3)::date as day,
				avg(extract(epoch from (resolved_at - created_at)) / 60.0) as avg_minutes,
				count(*) as sample_size
			from requests
			where workspace_id = $1
			  and resolved_at is not null
			  and resolved_at >= now() - make_interval(days => $2)
			group by 1
		)
		select
			to_char(days.day, 'YYYY-MM-DD') as date,
			ack.avg_minutes,
			coalesce(ack.sample_size, 0) as ack_sample_size,
			resolve.avg_minutes,
			coalesce(resolve.sample_size, 0) as resolve_sample_size
		from days
		left join ack on ack.day = days.day
		left join resolve on resolve.day = days.day
		order by days.day asc
	`, workspaceID, windowDays, timezone)
	if err != nil {
		return RequestAnalytics{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var point RequestTrendPoint
		if err := rows.Scan(
			&point.Date,
			&point.AckAvgMinutes,
			&point.AckSampleSize,
			&point.ResolveAvgMinutes,
			&point.ResolveSampleSize,
		); err != nil {
			return RequestAnalytics{}, err
		}
		out.Trend = append(out.Trend, point)
	}
	if err := rows.Err(); err != nil {
		return RequestAnalytics{}, err
	}

	return out, nil
}

func (s *Store) ListRequestReportRows(ctx context.Context, workspaceID uuid.UUID, windowDays int) ([]RequestReportRow, error) {
	if windowDays != 7 && windowDays != 30 && windowDays != 90 {
		windowDays = 30
	}
	rows, err := s.pool.Query(ctx, `
		select
			r.id,
			r.channel_id,
			sc.channel_name,
			r.thread_ts,
			case
				when si.team_domain is null or si.team_domain = '' then null
				else 'https://' || si.team_domain || '.slack.com/archives/' || r.channel_id || '/p' || replace(r.thread_ts, '.', '')
			end as thread_url,
			r.title,
			r.status,
			r.priority,
			r.owner_slack_id,
			r.created_at,
			r.acked_at,
			r.assigned_at,
			r.resolved_at,
			r.due_at,
			r.ack_overdue_sent_at,
			r.assign_overdue_sent_at,
			r.stale_sent_at,
			li.issue_id,
			li.issue_url
		from requests r
		left join linear_issues li on li.request_id = r.id
		left join slack_installations si on si.workspace_id = r.workspace_id
		left join slack_channels sc on sc.workspace_id = r.workspace_id and sc.channel_id = r.channel_id
		where r.workspace_id = $1
			and r.created_at >= now() - make_interval(days => $2)
		order by r.created_at desc
	`, workspaceID, windowDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RequestReportRow, 0)
	for rows.Next() {
		var item RequestReportRow
		if err := rows.Scan(
			&item.ID,
			&item.ChannelID,
			&item.ChannelName,
			&item.ThreadTS,
			&item.ThreadURL,
			&item.Title,
			&item.Status,
			&item.Priority,
			&item.OwnerSlackID,
			&item.CreatedAt,
			&item.AckedAt,
			&item.AssignedAt,
			&item.ResolvedAt,
			&item.DueAt,
			&item.AckOverdueSentAt,
			&item.AssignOverdueSentAt,
			&item.StaleSentAt,
			&item.LinearIssueID,
			&item.LinearIssueURL,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ListRequestsByStatus(ctx context.Context, workspaceID uuid.UUID, status string, limit int) ([]Request, error) {
	query := fmt.Sprintf(`
		select %s
		from requests
		where workspace_id = $1
	`, requestSelectColumns(""))
	args := []any{workspaceID}
	normalized := strings.ToUpper(strings.TrimSpace(status))
	if normalized != "" {
		if normalized == "OPEN" || normalized == "CLOSED" {
			query += " and state = $2"
		} else {
			query += " and status = $2"
		}
		args = append(args, normalized)
	}
	query += " order by created_at desc"
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if len(args) == 1 {
		query += " limit $2"
		args = append(args, limit)
	} else {
		query += " limit $3"
		args = append(args, limit)
	}

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]Request, 0)
	for rows.Next() {
		var r Request
		if err := scanRequest(rows, &r); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

func (s *Store) ListActivity(ctx context.Context, workspaceID uuid.UUID, limit int) ([]ActivityRow, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `
		select re.id, re.request_id, re.event_type, re.actor_type, re.actor_id, re.payload_json, re.occurred_at
		from request_events re
		join requests r on r.id = re.request_id
		where r.workspace_id = $1
		order by re.occurred_at desc
		limit $2
	`, workspaceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ActivityRow, 0)
	for rows.Next() {
		var a ActivityRow
		if err := rows.Scan(&a.ID, &a.RequestID, &a.ActionType, &a.ActorType, &a.ActorID, &a.Payload, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := s.pool.Query(ctx, `
		select workspace_id, ack_sla_minutes, assign_sla_minutes, stale_hours,
		to_char(daily_digest_time, 'HH24:MI:SS'), timezone, escalation_channel_id,
		last_digest_sent_at, linear_team_id
		from policies
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Policy, 0)
	for rows.Next() {
		var p Policy
		if err := rows.Scan(
			&p.WorkspaceID,
			&p.AckSLAMinutes,
			&p.AssignSLAMinutes,
			&p.StaleHours,
			&p.DailyDigestTime,
			&p.Timezone,
			&p.EscalationChannelID,
			&p.LastDigestSentAt,
			&p.LinearTeamID,
		); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type SLACandidate struct {
	Request  Request
	Policy   Policy
	TeamID   string
	Domain   string
	BotToken string
}

func (s *Store) FindAckOverdue(ctx context.Context) ([]SLACandidate, error) {
	rows, err := s.pool.Query(ctx, `
		select
			r.id, r.workspace_id, r.source_key, r.channel_id, r.thread_ts, r.message_ts,
			r.author_slack_id, r.body_text, r.title, r.status, r.priority, r.owner_slack_id,
			r.due_at, r.acked_at, r.assigned_at, r.resolved_at, r.last_activity_at,
			r.ack_overdue_sent_at, r.assign_overdue_sent_at, r.stale_sent_at, r.created_at,
			r.triage_message_ts,
			p.workspace_id, eff.ack_sla_minutes, eff.assign_sla_minutes, eff.stale_hours,
			to_char(p.daily_digest_time, 'HH24:MI:SS'), p.timezone, p.escalation_channel_id,
			p.last_digest_sent_at, p.linear_team_id,
			si.team_id, coalesce(si.team_domain, ''), si.bot_token
		from requests r
		join policies p on p.workspace_id = r.workspace_id
		join slack_installations si on si.workspace_id = r.workspace_id
		left join workspace_subscriptions ws on ws.workspace_id = r.workspace_id
		left join channel_sla_overrides cso on cso.workspace_id = r.workspace_id and cso.channel_id = r.channel_id
		left join lateral (
			select
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.ack_sla_minutes, p.ack_sla_minutes)
					else p.ack_sla_minutes
				end as ack_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.assign_sla_minutes, p.assign_sla_minutes)
					else p.assign_sla_minutes
				end as assign_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.stale_hours, p.stale_hours)
					else p.stale_hours
				end as stale_hours
		) eff on true
		where r.status = 'NEW'
		and r.ack_overdue_sent_at is null
		and now() > r.created_at + make_interval(mins => case when eff.ack_sla_minutes > 0 then eff.ack_sla_minutes else 15 end)
		limit 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSLACandidates(rows)
}

func (s *Store) FindAssignOverdue(ctx context.Context) ([]SLACandidate, error) {
	rows, err := s.pool.Query(ctx, `
		select
			r.id, r.workspace_id, r.source_key, r.channel_id, r.thread_ts, r.message_ts,
			r.author_slack_id, r.body_text, r.title, r.status, r.priority, r.owner_slack_id,
			r.due_at, r.acked_at, r.assigned_at, r.resolved_at, r.last_activity_at,
			r.ack_overdue_sent_at, r.assign_overdue_sent_at, r.stale_sent_at, r.created_at,
			r.triage_message_ts,
			p.workspace_id, eff.ack_sla_minutes, eff.assign_sla_minutes, eff.stale_hours,
			to_char(p.daily_digest_time, 'HH24:MI:SS'), p.timezone, p.escalation_channel_id,
			p.last_digest_sent_at, p.linear_team_id,
			si.team_id, coalesce(si.team_domain, ''), si.bot_token
		from requests r
		join policies p on p.workspace_id = r.workspace_id
		join slack_installations si on si.workspace_id = r.workspace_id
		left join workspace_subscriptions ws on ws.workspace_id = r.workspace_id
		left join channel_sla_overrides cso on cso.workspace_id = r.workspace_id and cso.channel_id = r.channel_id
		left join lateral (
			select
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.ack_sla_minutes, p.ack_sla_minutes)
					else p.ack_sla_minutes
				end as ack_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.assign_sla_minutes, p.assign_sla_minutes)
					else p.assign_sla_minutes
				end as assign_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.stale_hours, p.stale_hours)
					else p.stale_hours
				end as stale_hours
		) eff on true
		where r.status in ('NEW','ACKED')
		and r.owner_slack_id is null
		and r.assign_overdue_sent_at is null
		and now() > r.created_at + make_interval(mins => case when eff.assign_sla_minutes > 0 then eff.assign_sla_minutes else 30 end)
		limit 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSLACandidates(rows)
}

func (s *Store) FindStale(ctx context.Context) ([]SLACandidate, error) {
	rows, err := s.pool.Query(ctx, `
		select
			r.id, r.workspace_id, r.source_key, r.channel_id, r.thread_ts, r.message_ts,
			r.author_slack_id, r.body_text, r.title, r.status, r.priority, r.owner_slack_id,
			r.due_at, r.acked_at, r.assigned_at, r.resolved_at, r.last_activity_at,
			r.ack_overdue_sent_at, r.assign_overdue_sent_at, r.stale_sent_at, r.created_at,
			r.triage_message_ts,
			p.workspace_id, eff.ack_sla_minutes, eff.assign_sla_minutes, eff.stale_hours,
			to_char(p.daily_digest_time, 'HH24:MI:SS'), p.timezone, p.escalation_channel_id,
			p.last_digest_sent_at, p.linear_team_id,
			si.team_id, coalesce(si.team_domain, ''), si.bot_token
		from requests r
		join policies p on p.workspace_id = r.workspace_id
		join slack_installations si on si.workspace_id = r.workspace_id
		left join workspace_subscriptions ws on ws.workspace_id = r.workspace_id
		left join channel_sla_overrides cso on cso.workspace_id = r.workspace_id and cso.channel_id = r.channel_id
		left join lateral (
			select
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.ack_sla_minutes, p.ack_sla_minutes)
					else p.ack_sla_minutes
				end as ack_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.assign_sla_minutes, p.assign_sla_minutes)
					else p.assign_sla_minutes
				end as assign_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.stale_hours, p.stale_hours)
					else p.stale_hours
				end as stale_hours
		) eff on true
		where r.status in ('ACKED','ASSIGNED')
		and r.stale_sent_at is null
		and now() > r.last_activity_at + make_interval(hours => case when eff.stale_hours > 0 then eff.stale_hours else 24 end)
		limit 200
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanSLACandidates(rows)
}

func (s *Store) scanSLACandidates(rows pgx.Rows) ([]SLACandidate, error) {
	out := make([]SLACandidate, 0)
	for rows.Next() {
		var item SLACandidate
		if err := rows.Scan(
			&item.Request.ID,
			&item.Request.WorkspaceID,
			&item.Request.SourceKey,
			&item.Request.ChannelID,
			&item.Request.ThreadTS,
			&item.Request.MessageTS,
			&item.Request.AuthorSlackID,
			&item.Request.BodyText,
			&item.Request.Title,
			&item.Request.Status,
			&item.Request.Priority,
			&item.Request.OwnerSlackID,
			&item.Request.DueAt,
			&item.Request.AckedAt,
			&item.Request.AssignedAt,
			&item.Request.ResolvedAt,
			&item.Request.LastActivityAt,
			&item.Request.AckOverdueSentAt,
			&item.Request.AssignOverdueSentAt,
			&item.Request.StaleSentAt,
			&item.Request.CreatedAt,
			&item.Request.TriageMessageTS,
			&item.Policy.WorkspaceID,
			&item.Policy.AckSLAMinutes,
			&item.Policy.AssignSLAMinutes,
			&item.Policy.StaleHours,
			&item.Policy.DailyDigestTime,
			&item.Policy.Timezone,
			&item.Policy.EscalationChannelID,
			&item.Policy.LastDigestSentAt,
			&item.Policy.LinearTeamID,
			&item.TeamID,
			&item.Domain,
			&item.BotToken,
		); err != nil {
			return nil, err
		}
		token, err := s.decryptToken(item.BotToken)
		if err != nil {
			return nil, err
		}
		item.BotToken = token
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) MarkAckOverdueSent(ctx context.Context, requestID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `update requests set ack_overdue_sent_at = now() where id = $1`, requestID)
	return err
}

func (s *Store) MarkAssignOverdueSent(ctx context.Context, requestID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `update requests set assign_overdue_sent_at = now() where id = $1`, requestID)
	return err
}

func (s *Store) MarkStaleSent(ctx context.Context, requestID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `update requests set stale_sent_at = now() where id = $1`, requestID)
	return err
}

func (s *Store) ShouldSendDigest(ctx context.Context, workspaceID uuid.UUID, now time.Time, timezone string, dailyDigestTime string, lastSent *time.Time) (bool, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	t, err := time.Parse("15:04:05", dailyDigestTime)
	if err != nil {
		return false, fmt.Errorf("parse digest time: %w", err)
	}
	target := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), t.Hour(), t.Minute(), 0, 0, loc)
	if localNow.Before(target) {
		return false, nil
	}
	if lastSent == nil {
		return true, nil
	}
	lastLocal := lastSent.In(loc)
	if lastLocal.Year() == localNow.Year() && lastLocal.Month() == localNow.Month() && lastLocal.Day() == localNow.Day() {
		return false, nil
	}
	return true, nil
}

func (s *Store) DigestPayload(ctx context.Context, workspaceID uuid.UUID) (int, int, int, []OverdueRow, error) {
	var overdueAck int
	var unassigned int
	var p0Active int
	err := s.pool.QueryRow(ctx, `
		select
			count(*) filter (
				where r.status = 'NEW'
				and now() > r.created_at + make_interval(mins => case when eff.ack_sla_minutes > 0 then eff.ack_sla_minutes else 15 end)
			) as overdue_ack,
			count(*) filter (
				where r.owner_slack_id is null
				and r.status in ('NEW','ACKED')
				and now() > r.created_at + make_interval(mins => case when eff.assign_sla_minutes > 0 then eff.assign_sla_minutes else 30 end)
			) as unassigned,
			count(*) filter (
				where r.priority = 'P0'
				and r.status not in ('RESOLVED','IGNORED')
			) as p0_active
		from requests r
		join policies p on p.workspace_id = r.workspace_id
		left join workspace_subscriptions ws on ws.workspace_id = r.workspace_id
		left join channel_sla_overrides cso on cso.workspace_id = r.workspace_id and cso.channel_id = r.channel_id
		left join lateral (
			select
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.ack_sla_minutes, p.ack_sla_minutes)
					else p.ack_sla_minutes
				end as ack_sla_minutes,
				case
					when lower(coalesce(ws.plan_key, '')) in ('enterprise', 'pro')
						and lower(coalesce(ws.status, '')) in ('active', 'trialing', 'past_due', 'unpaid', 'incomplete')
					then coalesce(cso.assign_sla_minutes, p.assign_sla_minutes)
					else p.assign_sla_minutes
				end as assign_sla_minutes
		) eff on true
		where r.workspace_id = $1
	`, workspaceID).Scan(&overdueAck, &unassigned, &p0Active)
	if err != nil {
		return 0, 0, 0, nil, err
	}

	rows, err := s.pool.Query(ctx, `
		select id, status, priority, title, channel_id, thread_ts, owner_slack_id, created_at, due_at
		from requests
		where workspace_id = $1
		and status not in ('RESOLVED','IGNORED')
		order by
			case when priority = 'P0' then 0 when priority = 'P1' then 1 else 2 end,
			created_at asc
		limit 10
	`, workspaceID)
	if err != nil {
		return 0, 0, 0, nil, err
	}
	defer rows.Close()

	list := make([]OverdueRow, 0)
	for rows.Next() {
		var r OverdueRow
		if err := rows.Scan(&r.ID, &r.Status, &r.Priority, &r.Title, &r.ChannelID, &r.ThreadTS, &r.OwnerSlackID, &r.CreatedAt, &r.DueAt); err != nil {
			return 0, 0, 0, nil, err
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, nil, err
	}
	return overdueAck, unassigned, p0Active, list, nil
}

func (s *Store) MarkDigestSent(ctx context.Context, workspaceID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `update policies set last_digest_sent_at = now() where workspace_id = $1`, workspaceID)
	return err
}
