package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
	"triageguard/apps/api/internal/linear"
)

const linearRefreshLeeway = 2 * time.Minute

func linearTokenExpiresAt(now time.Time, expiresIn int) *time.Time {
	if expiresIn <= 0 {
		return nil
	}
	expiresAt := now.UTC().Add(time.Duration(expiresIn) * time.Second)
	return &expiresAt
}

func shouldRefreshLinearToken(install db.LinearInstallation, now time.Time) bool {
	if install.TokenExpiresAt == nil {
		return false
	}
	return install.TokenExpiresAt.Before(now.UTC().Add(linearRefreshLeeway))
}

func (a *App) refreshLinearInstallation(ctx context.Context, workspaceID uuid.UUID, install db.LinearInstallation) (db.LinearInstallation, error) {
	if install.RefreshToken == nil || strings.TrimSpace(*install.RefreshToken) == "" {
		return db.LinearInstallation{}, fmt.Errorf("%w: refresh token is missing", linear.ErrNotAuthenticated)
	}
	if strings.TrimSpace(a.cfg.LinearClientID) == "" || strings.TrimSpace(a.cfg.LinearClientSecret) == "" {
		return db.LinearInstallation{}, fmt.Errorf("linear oauth client is not configured")
	}

	refreshed, err := a.linearClient.RefreshOAuthToken(ctx, a.cfg.LinearClientID, a.cfg.LinearClientSecret, *install.RefreshToken)
	if err != nil {
		return db.LinearInstallation{}, err
	}

	if err := a.store.UpsertLinearInstallation(
		ctx,
		workspaceID,
		refreshed.AccessToken,
		optionalString(refreshed.RefreshToken),
		linearTokenExpiresAt(time.Now(), refreshed.ExpiresIn),
	); err != nil {
		return db.LinearInstallation{}, err
	}

	return a.store.GetLinearInstallation(ctx, workspaceID)
}

func (a *App) withLinearAccessToken(ctx context.Context, workspaceID uuid.UUID, operation func(token string) error) error {
	install, err := a.store.GetLinearInstallation(ctx, workspaceID)
	if err != nil {
		return err
	}

	now := time.Now()
	if shouldRefreshLinearToken(install, now) {
		refreshed, refreshErr := a.refreshLinearInstallation(ctx, workspaceID, install)
		if refreshErr != nil {
			if linear.IsAuthError(refreshErr) {
				return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
			}
			return refreshErr
		}
		install = refreshed
	}

	runErr := operation(install.AccessToken)
	if runErr == nil {
		return nil
	}
	if !linear.IsAuthError(runErr) {
		return runErr
	}

	refreshed, refreshErr := a.refreshLinearInstallation(ctx, workspaceID, install)
	if refreshErr != nil {
		if linear.IsAuthError(refreshErr) {
			return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
		}
		return refreshErr
	}
	retryErr := operation(refreshed.AccessToken)
	if retryErr != nil {
		if linear.IsAuthError(retryErr) {
			return fmt.Errorf("linear auth required: reconnect Linear in Admin Console")
		}
		return retryErr
	}
	return nil
}
