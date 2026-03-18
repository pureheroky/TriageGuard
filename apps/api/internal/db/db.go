package db

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultConnectTimeout = 10 * time.Second

func NewPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse db url: %w", err)
	}

	cfg.MaxConns = 12
	if cfg.ConnConfig.ConnectTimeout <= 0 {
		cfg.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}
	if shouldUseSimpleProtocol(cfg.ConnConfig.Host) {
		// Supabase Session Pooler (PgBouncer transaction mode) is not compatible
		// with prepared statement caching used by pgx defaults.
		cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
		cfg.ConnConfig.StatementCacheCapacity = 0
		cfg.ConnConfig.DescriptionCacheCapacity = 0
	}
	if shouldUseIPv4First(cfg.ConnConfig.Host) {
		cfg.ConnConfig.DialFunc = newIPv4FirstDialFunc(cfg.ConnConfig.ConnectTimeout)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, withSupabaseDBHint(err, cfg.ConnConfig.Host)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnConfig.ConnectTimeout)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, withSupabaseDBHint(err, cfg.ConnConfig.Host)
	}

	return pool, nil
}

func DBHostFromDSN(dsn string) string {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "unknown"
	}
	host := strings.TrimSpace(cfg.ConnConfig.Host)
	if host == "" {
		return "unknown"
	}
	return host
}

func shouldUseIPv4First(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	return strings.HasSuffix(host, ".supabase.co")
}

func shouldUseSimpleProtocol(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return false
	}
	return strings.Contains(host, ".pooler.supabase.com")
}

func newIPv4FirstDialFunc(timeout time.Duration) func(context.Context, string, string) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err4 := dialer.DialContext(ctx, "tcp4", address)
		if err4 == nil {
			return conn, nil
		}
		conn, errAny := dialer.DialContext(ctx, "tcp", address)
		if errAny == nil {
			return conn, nil
		}
		return nil, fmt.Errorf("dial tcp4 failed: %w; dial tcp failed: %w", err4, errAny)
	}
}

func withSupabaseDBHint(err error, host string) error {
	if err == nil {
		return nil
	}
	host = strings.ToLower(strings.TrimSpace(host))
	message := strings.ToLower(err.Error())

	if strings.HasPrefix(host, "db.") && strings.HasSuffix(host, ".supabase.co") &&
		(strings.Contains(message, "no route to host") ||
			strings.Contains(message, "network is unreachable") ||
			strings.Contains(message, "dial tcp [")) {
		return fmt.Errorf("%w. hint: this looks like an IPv6 connectivity issue for direct Supabase DB host (%s). Use the Supabase Session Pooler connection string (*.pooler.supabase.com) in SUPABASE_DB_URL", err, host)
	}

	return err
}
