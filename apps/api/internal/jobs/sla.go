package jobs

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
	"triageguard/apps/api/internal/slack"
)

type SLARunner struct {
	cfg          config.Config
	store        *db.Store
	slackClient  *slack.Client
	linearClient *linear.Client
}

const slaRunnerLockKey int64 = 81273451

func NewSLARunner(cfg config.Config, store *db.Store, slackClient *slack.Client, linearClient *linear.Client) *SLARunner {
	return &SLARunner{cfg: cfg, store: store, slackClient: slackClient, linearClient: linearClient}
}

func (r *SLARunner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	if err := r.RunOnce(ctx); err != nil {
		log.Printf("sla: initial run failed: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.RunOnce(ctx); err != nil {
				log.Printf("sla: periodic run failed: %v", err)
			}
		}
	}
}

func (r *SLARunner) RunOnce(ctx context.Context) error {
	locked, err := r.store.TryAdvisoryLock(ctx, slaRunnerLockKey)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.store.AdvisoryUnlock(unlockCtx, slaRunnerLockKey); err != nil {
			log.Printf("sla: advisory unlock failed: %v", err)
		}
	}()

	paidAccessCache := map[uuid.UUID]bool{}
	if err := r.runAckOverdue(ctx, paidAccessCache); err != nil {
		return err
	}
	if err := r.runAssignOverdue(ctx, paidAccessCache); err != nil {
		return err
	}
	if err := r.runStale(ctx, paidAccessCache); err != nil {
		return err
	}
	if err := r.runDigest(ctx, paidAccessCache); err != nil {
		return err
	}
	return nil
}

func (r *SLARunner) workspaceHasPaidAccess(ctx context.Context, workspaceID uuid.UUID, cache map[uuid.UUID]bool) (bool, error) {
	if allowed, ok := cache[workspaceID]; ok {
		return allowed, nil
	}
	sub, err := r.store.GetWorkspaceSubscriptionOrDefault(ctx, workspaceID)
	if err != nil {
		return false, err
	}
	status := strings.ToLower(strings.TrimSpace(sub.Status))
	isPaidStatus := status == "active" || status == "trialing" || status == "past_due" || status == "unpaid" || status == "incomplete"
	plan := strings.ToLower(strings.TrimSpace(sub.PlanKey))
	if plan == "pro" {
		plan = "enterprise"
	}
	if plan == "starter" {
		plan = "team"
	}
	allowed := (plan == "team" || plan == "enterprise") && isPaidStatus
	cache[workspaceID] = allowed
	return allowed, nil
}

func (r *SLARunner) runAckOverdue(ctx context.Context, paidAccessCache map[uuid.UUID]bool) error {
	items, err := r.store.FindAckOverdue(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		allowed, err := r.workspaceHasPaidAccess(ctx, item.Request.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: ack overdue paid access check failed workspace=%s: %v", item.Request.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}
		threadURL := buildThreadURL(item.Domain, item.Request.ChannelID, item.Request.ThreadTS)
		threadSent := true
		if _, err := r.slackClient.PostMessage(ctx, item.BotToken, item.Request.ChannelID, "⚠️ Ack overdue", item.Request.ThreadTS, nil); err != nil {
			threadSent = false
			log.Printf("sla: ack overdue thread send failed request_id=%s: %v", item.Request.ID, err)
		}
		escalationSent := true
		if item.Policy.EscalationChannelID != nil {
			text := "Overdue Ack"
			if threadURL != "" {
				text = fmt.Sprintf("Overdue Ack: <%s|open thread>", threadURL)
			}
			if _, err := r.slackClient.PostMessage(ctx, item.BotToken, *item.Policy.EscalationChannelID, text, "", nil); err != nil {
				escalationSent = false
				log.Printf("sla: ack overdue escalation send failed request_id=%s: %v", item.Request.ID, err)
			}
		}
		if threadSent && escalationSent {
			if err := r.store.MarkAckOverdueSent(ctx, item.Request.ID); err != nil {
				log.Printf("sla: ack overdue mark sent failed request_id=%s: %v", item.Request.ID, err)
			}
		}
	}
	return nil
}

func (r *SLARunner) runAssignOverdue(ctx context.Context, paidAccessCache map[uuid.UUID]bool) error {
	items, err := r.store.FindAssignOverdue(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		allowed, err := r.workspaceHasPaidAccess(ctx, item.Request.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: assign overdue paid access check failed workspace=%s: %v", item.Request.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}
		threadURL := buildThreadURL(item.Domain, item.Request.ChannelID, item.Request.ThreadTS)
		threadSent := true
		if _, err := r.slackClient.PostMessage(ctx, item.BotToken, item.Request.ChannelID, "⚠️ Assign overdue", item.Request.ThreadTS, nil); err != nil {
			threadSent = false
			log.Printf("sla: assign overdue thread send failed request_id=%s: %v", item.Request.ID, err)
		}
		escalationSent := true
		if item.Policy.EscalationChannelID != nil {
			text := "Overdue Assign"
			if threadURL != "" {
				text = fmt.Sprintf("Overdue Assign: <%s|open thread>", threadURL)
			}
			if _, err := r.slackClient.PostMessage(ctx, item.BotToken, *item.Policy.EscalationChannelID, text, "", nil); err != nil {
				escalationSent = false
				log.Printf("sla: assign overdue escalation send failed request_id=%s: %v", item.Request.ID, err)
			}
		}
		if threadSent && escalationSent {
			if err := r.store.MarkAssignOverdueSent(ctx, item.Request.ID); err != nil {
				log.Printf("sla: assign overdue mark sent failed request_id=%s: %v", item.Request.ID, err)
			}
		}
	}
	return nil
}

func (r *SLARunner) runStale(ctx context.Context, paidAccessCache map[uuid.UUID]bool) error {
	items, err := r.store.FindStale(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		allowed, err := r.workspaceHasPaidAccess(ctx, item.Request.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: stale paid access check failed workspace=%s: %v", item.Request.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}
		msg := "⚠️ Request is stale"
		if item.Request.OwnerSlackID != nil && *item.Request.OwnerSlackID != "" {
			msg = fmt.Sprintf("⚠️ Stale request ping <@%s>", *item.Request.OwnerSlackID)
		}
		threadSent := true
		if _, err := r.slackClient.PostMessage(ctx, item.BotToken, item.Request.ChannelID, msg, item.Request.ThreadTS, nil); err != nil {
			threadSent = false
			log.Printf("sla: stale thread send failed request_id=%s: %v", item.Request.ID, err)
		}
		escalationSent := true
		if item.Policy.EscalationChannelID != nil {
			threadURL := buildThreadURL(item.Domain, item.Request.ChannelID, item.Request.ThreadTS)
			esc := "Stale request"
			if threadURL != "" {
				esc = fmt.Sprintf("Stale request: <%s|open thread>", threadURL)
			}
			if _, err := r.slackClient.PostMessage(ctx, item.BotToken, *item.Policy.EscalationChannelID, esc, "", nil); err != nil {
				escalationSent = false
				log.Printf("sla: stale escalation send failed request_id=%s: %v", item.Request.ID, err)
			}
		}
		if threadSent && escalationSent {
			if err := r.store.MarkStaleSent(ctx, item.Request.ID); err != nil {
				log.Printf("sla: stale mark sent failed request_id=%s: %v", item.Request.ID, err)
			}
		}
	}
	return nil
}

func (r *SLARunner) runDigest(ctx context.Context, paidAccessCache map[uuid.UUID]bool) error {
	policies, err := r.store.ListPolicies(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, p := range policies {
		allowed, err := r.workspaceHasPaidAccess(ctx, p.WorkspaceID, paidAccessCache)
		if err != nil {
			log.Printf("sla: digest paid access check failed workspace=%s: %v", p.WorkspaceID, err)
			continue
		}
		if !allowed {
			continue
		}
		if p.EscalationChannelID == nil || *p.EscalationChannelID == "" {
			continue
		}
		ok, err := r.store.ShouldSendDigest(ctx, p.WorkspaceID, now, p.Timezone, p.DailyDigestTime, p.LastDigestSentAt)
		if err != nil || !ok {
			continue
		}
		install, err := r.store.GetSlackInstallationByWorkspace(ctx, p.WorkspaceID)
		if err != nil {
			continue
		}
		overdueAck, unassigned, p0, top, err := r.store.DigestPayload(ctx, p.WorkspaceID)
		if err != nil {
			continue
		}
		lines := []string{
			"*Daily TriageGuard Digest*",
			fmt.Sprintf("- Overdue Ack: %d", overdueAck),
			fmt.Sprintf("- Unassigned: %d", unassigned),
			fmt.Sprintf("- Active P0: %d", p0),
			"- Top requests:",
		}
		for _, req := range top {
			label := req.Priority
			if req.Title != nil && *req.Title != "" {
				label = label + " - " + *req.Title
			}
			threadURL := buildThreadURL(install.TeamDomain, req.ChannelID, req.ThreadTS)
			if threadURL != "" {
				lines = append(lines, fmt.Sprintf("  - <%s|%s>", threadURL, label))
			} else {
				lines = append(lines, fmt.Sprintf("  - #%s (%s)", req.ChannelID, label))
			}
		}
		if _, err := r.slackClient.PostMessage(ctx, install.BotToken, *p.EscalationChannelID, strings.Join(lines, "\n"), "", nil); err != nil {
			log.Printf("sla: digest send failed workspace=%s: %v", p.WorkspaceID, err)
			continue
		}
		if err := r.store.MarkDigestSent(ctx, p.WorkspaceID); err != nil {
			log.Printf("sla: digest mark sent failed workspace=%s: %v", p.WorkspaceID, err)
		}
	}
	return nil
}

func buildThreadURL(domain, channelID, threadTS string) string {
	ts := strings.ReplaceAll(threadTS, ".", "")
	if domain == "" || channelID == "" || ts == "" {
		return ""
	}
	return fmt.Sprintf("https://%s.slack.com/archives/%s/p%s", domain, channelID, ts)
}
