package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"triageguard/apps/api/internal/config"
	authmw "triageguard/apps/api/internal/middleware"
)

func TestIsInternalOperatorUser(t *testing.T) {
	allowedID := uuid.New()
	app := &App{
		cfg: config.Config{
			InternalOperatorUserIDs: []string{" " + allowedID.String() + " ", strings.ToUpper(allowedID.String())},
		},
	}

	if !app.isInternalOperatorUser(allowedID) {
		t.Fatalf("expected allowlisted user to be recognized as internal operator")
	}
	if app.isInternalOperatorUser(uuid.New()) {
		t.Fatalf("unexpected internal operator match for unrelated user")
	}
}

func TestRequireInternalOperator(t *testing.T) {
	allowedID := uuid.New()
	app := &App{cfg: config.Config{InternalOperatorUserIDs: []string{allowedID.String()}}}
	handler := app.requireInternalOperator(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	t.Run("missing claims is unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/ops/summary", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("non allowlisted user is forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/ops/summary", nil)
		req = req.WithContext(context.WithValue(req.Context(), authmw.ClaimsKey, jwt.MapClaims{"sub": uuid.New().String()}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusForbidden)
		}
	})

	t.Run("allowlisted user passes", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/ops/summary", nil)
		req = req.WithContext(context.WithValue(req.Context(), authmw.ClaimsKey, jwt.MapClaims{"sub": allowedID.String()}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("unexpected status: got %d want %d", rec.Code, http.StatusNoContent)
		}
	})
}
