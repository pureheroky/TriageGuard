package jobs

import (
	"context"
	"log"
	"time"

	"triageguard/apps/api/internal/config"
	"triageguard/apps/api/internal/db"
)

const retentionRunnerLockKey int64 = 81273452
const retentionMaxBatchLoops = 5000

type RetentionRunner struct {
	cfg   config.Config
	store *db.Store
}

func NewRetentionRunner(cfg config.Config, store *db.Store) *RetentionRunner {
	return &RetentionRunner{cfg: cfg, store: store}
}

func (r *RetentionRunner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Hour
	}
	if err := r.RunOnce(ctx); err != nil {
		log.Printf("retention: initial run failed: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.RunOnce(ctx); err != nil {
				log.Printf("retention: periodic run failed: %v", err)
			}
		}
	}
}

func (r *RetentionRunner) RunOnce(ctx context.Context) error {
	if !r.cfg.RetentionEnabled {
		return nil
	}

	locked, err := r.store.TryAdvisoryLock(ctx, retentionRunnerLockKey)
	if err != nil {
		return err
	}
	if !locked {
		return nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := r.store.AdvisoryUnlock(unlockCtx, retentionRunnerLockKey); err != nil {
			log.Printf("retention: advisory unlock failed: %v", err)
		}
	}()

	now := time.Now().UTC()
	terminalCutoff := now.AddDate(0, 0, -r.cfg.RetentionTerminalDays)
	dedupeCutoff := now.AddDate(0, 0, -r.cfg.RetentionDedupeDays)

	var totalTerminal int64
	for i := 0; i < retentionMaxBatchLoops; i++ {
		deleted, err := r.store.DeleteTerminalRequestsBefore(ctx, terminalCutoff, r.cfg.RetentionBatchSize)
		if err != nil {
			return err
		}
		totalTerminal += deleted
		if deleted == 0 || deleted < int64(r.cfg.RetentionBatchSize) {
			break
		}
		if i == retentionMaxBatchLoops-1 {
			log.Printf("retention: reached max loops while deleting terminal requests; deleted_so_far=%d", totalTerminal)
		}
	}

	deletedSlackDedup, err := r.store.DeleteSlackActionDedupBefore(ctx, dedupeCutoff)
	if err != nil {
		return err
	}
	deletedStripeEvents, err := r.store.DeleteStripeWebhookEventsBefore(ctx, dedupeCutoff)
	if err != nil {
		return err
	}
	deletedLinearEvents, err := r.store.DeleteLinearWebhookEventsBefore(ctx, dedupeCutoff)
	if err != nil {
		return err
	}

	if totalTerminal > 0 || deletedSlackDedup > 0 || deletedStripeEvents > 0 || deletedLinearEvents > 0 {
		log.Printf(
			"retention: deleted terminal_requests=%d slack_action_dedup=%d stripe_webhook_events=%d linear_webhook_events=%d terminal_cutoff=%s dedupe_cutoff=%s",
			totalTerminal,
			deletedSlackDedup,
			deletedStripeEvents,
			deletedLinearEvents,
			terminalCutoff.Format(time.RFC3339),
			dedupeCutoff.Format(time.RFC3339),
		)
	}
	return nil
}
