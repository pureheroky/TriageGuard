package handlers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"

	"triageguard/apps/api/internal/db"
)

func (a *App) recordDeadLetter(
	ctx context.Context,
	source string,
	operation string,
	workspaceID *uuid.UUID,
	requestID *uuid.UUID,
	externalID *string,
	err error,
	payload map[string]any,
) {
	if err == nil {
		return
	}

	rawPayload := json.RawMessage(`{}`)
	if len(payload) > 0 {
		if encoded, marshalErr := json.Marshal(payload); marshalErr == nil {
			rawPayload = encoded
		}
	}

	item := db.DeadLetter{
		Source:       strings.TrimSpace(source),
		Operation:    strings.TrimSpace(operation),
		WorkspaceID:  workspaceID,
		RequestID:    requestID,
		ExternalID:   externalID,
		ErrorMessage: strings.TrimSpace(err.Error()),
		Payload:      rawPayload,
	}

	if insertErr := a.store.InsertDeadLetter(ctx, item); insertErr != nil {
		a.logger.Printf("dead-letter insert failed source=%s operation=%s: %v", source, operation, insertErr)
		return
	}

	if a.alerts != nil {
		a.alerts.RecordDeadLetter(source, operation, err.Error())
	}
	if a.sentry != nil && a.sentry.Enabled() {
		captureCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = a.sentry.Capture(captureCtx, "error", "dead letter recorded", map[string]string{
			"source":    strings.TrimSpace(source),
			"operation": strings.TrimSpace(operation),
		}, map[string]any{
			"workspace_id": uuidString(workspaceID),
			"request_id":   uuidString(requestID),
			"external_id":  stringOrEmpty(externalID),
			"error":        strings.TrimSpace(err.Error()),
		})
	}
}

func uuidString(v *uuid.UUID) string {
	if v == nil {
		return ""
	}
	return v.String()
}

func stringOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(*v)
}
