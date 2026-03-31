package handlers

import (
	"net/http"
	"strconv"

	"triageguard/apps/api/internal/db"
)

func parseOptionalLimit(raw string, fallback, max int) int {
	if fallback <= 0 {
		fallback = 50
	}
	if max <= 0 {
		max = 500
	}
	if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
		if parsed > max {
			return max
		}
		return parsed
	}
	return fallback
}

func (a *App) handleSeedSampleData(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	userID, _ := userIDFromContext(r.Context())
	hasEnterprise, err := a.workspaceHasEnterprise(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	var sharedPolicy *db.Policy
	if !hasEnterprise {
		policy, err := a.store.GetPolicy(r.Context(), workspace.ID)
		if err != nil {
			jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		sharedPolicy = &policy
	}
	if err := a.store.SeedDemoWorkspace(r.Context(), workspace.ID, &userID, sharedPolicy); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "seeded": true})
}

func (a *App) handleResetSampleData(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	if err := a.store.ResetDemoWorkspace(r.Context(), workspace.ID); err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"ok": true, "reset": true})
}

func (a *App) handleWorkspaceOpsSummary(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	summary, err := a.store.WorkspaceOpsSummary(r.Context(), workspace.ID)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, summary)
}

func (a *App) handleWorkspaceOpsEvents(w http.ResponseWriter, r *http.Request) {
	workspace, err := a.workspaceFromContext(r)
	if err != nil {
		jsonResponse(w, http.StatusNotFound, map[string]any{"error": "workspace not found"})
		return
	}
	limit := parseOptionalLimit(r.URL.Query().Get("limit"), 50, 250)
	deadLetters, err := a.store.ListDeadLetters(r.Context(), workspace.ID, limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	outbox, err := a.store.ListWorkspaceOutboxItems(r.Context(), workspace.ID, limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"dead_letters": deadLetters,
		"outbox":       outbox,
	})
}

func (a *App) handleInternalOpsSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := a.store.InternalOpsSummary(r.Context())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, summary)
}

func (a *App) handleInternalDeadLetters(w http.ResponseWriter, r *http.Request) {
	limit := parseOptionalLimit(r.URL.Query().Get("limit"), 100, 500)
	rows, err := a.store.ListDeadLettersAll(r.Context(), limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"dead_letters": rows})
}

func (a *App) handleInternalOutbox(w http.ResponseWriter, r *http.Request) {
	limit := parseOptionalLimit(r.URL.Query().Get("limit"), 100, 500)
	rows, err := a.store.ListOutboxItemsAll(r.Context(), limit)
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"outbox": rows})
}

func (a *App) handleInternalRunnerStatus(w http.ResponseWriter, r *http.Request) {
	rows, err := a.store.ListJobRuntimeStatuses(r.Context())
	if err != nil {
		jsonResponse(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{"runners": rows})
}
