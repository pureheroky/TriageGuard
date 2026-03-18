package handlers

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	oauthStateTTL     = 15 * time.Minute
	oauthCookiePrefix = "tg_oauth_state_"
)

type oauthStatePayload struct {
	Provider    string `json:"p"`
	Nonce       string `json:"n"`
	IssuedAt    int64  `json:"iat"`
	UserID      string `json:"u,omitempty"`
	WorkspaceID string `json:"w,omitempty"`
}

func (a *App) buildOAuthState(provider string, userID, workspaceID *uuid.UUID) (state string, nonce string, err error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return "", "", err
	}
	nonce = hex.EncodeToString(nonceBytes)

	payload := oauthStatePayload{
		Provider: provider,
		Nonce:    nonce,
		IssuedAt: time.Now().UTC().Unix(),
	}
	if userID != nil && *userID != uuid.Nil {
		payload.UserID = userID.String()
	}
	if workspaceID != nil && *workspaceID != uuid.Nil {
		payload.WorkspaceID = workspaceID.String()
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}
	payloadB64 := base64.RawURLEncoding.EncodeToString(rawPayload)

	mac := hmac.New(sha256.New, []byte(a.cfg.OAuthStateSecret))
	_, _ = mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nonce, nil
}

func (a *App) parseOAuthState(provider, state string) (oauthStatePayload, error) {
	parts := strings.Split(state, ".")
	if len(parts) != 2 {
		return oauthStatePayload{}, errors.New("invalid oauth state format")
	}
	payloadB64 := parts[0]
	sigB64 := parts[1]

	mac := hmac.New(sha256.New, []byte(a.cfg.OAuthStateSecret))
	_, _ = mac.Write([]byte(payloadB64))
	expectedSigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sigB64), []byte(expectedSigB64)) != 1 {
		return oauthStatePayload{}, errors.New("invalid oauth state signature")
	}

	rawPayload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return oauthStatePayload{}, errors.New("invalid oauth state payload")
	}

	var payload oauthStatePayload
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return oauthStatePayload{}, errors.New("invalid oauth state json")
	}
	if payload.Provider != provider {
		return oauthStatePayload{}, errors.New("oauth provider mismatch")
	}
	if payload.Nonce == "" {
		return oauthStatePayload{}, errors.New("missing oauth nonce")
	}

	now := time.Now().UTC().Unix()
	if payload.IssuedAt <= 0 || now-payload.IssuedAt > int64(oauthStateTTL.Seconds()) || payload.IssuedAt-now > 300 {
		return oauthStatePayload{}, errors.New("oauth state expired")
	}

	return payload, nil
}

func oauthStateCookieName(provider string) string {
	return oauthCookiePrefix + provider
}

func (a *App) setOAuthStateCookie(w http.ResponseWriter, provider, nonce string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName(provider),
		Value:    nonce,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureCookie(a.cfg.APIBaseURL),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(oauthStateTTL.Seconds()),
	})
}

func (a *App) clearOAuthStateCookie(w http.ResponseWriter, provider string) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookieName(provider),
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureCookie(a.cfg.APIBaseURL),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func (a *App) validateOAuthState(r *http.Request, provider string) (oauthStatePayload, error) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	if state == "" {
		return oauthStatePayload{}, errors.New("missing oauth state")
	}

	payload, err := a.parseOAuthState(provider, state)
	if err != nil {
		return oauthStatePayload{}, err
	}

	cookie, err := r.Cookie(oauthStateCookieName(provider))
	if err == nil {
		if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(cookie.Value)), []byte(payload.Nonce)) != 1 {
			return oauthStatePayload{}, errors.New("oauth nonce mismatch")
		}
	}
	// Cookie may be absent in local/proxy flows where OAuth is initiated on one host
	// (e.g. localhost web BFF) and callback lands on API host (e.g. ngrok).
	// In this case we rely on signed+expiring state value.
	if err != nil && !errors.Is(err, http.ErrNoCookie) {
		return oauthStatePayload{}, errors.New("invalid oauth state cookie")
	}

	return payload, nil
}

func isSecureCookie(baseURL string) bool {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Scheme, "https")
}
