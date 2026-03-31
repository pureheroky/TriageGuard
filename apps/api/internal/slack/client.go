package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://slack.com/api"
const slackRequestMaxAttempts = 3
const slackRetryBaseDelay = 300 * time.Millisecond
const slackRetryMaxDelay = 5 * time.Second

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type PostMessageResponse struct {
	OK      bool   `json:"ok"`
	TS      string `json:"ts"`
	Channel string `json:"channel"`
	Error   string `json:"error"`
}

type Conversation struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsPrivate bool   `json:"is_private"`
}

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	RealName    string `json:"real_name"`
	Email       string `json:"email"`
}

func (c *Client) PostMessage(ctx context.Context, token, channel, text, threadTS string, blocks any) (PostMessageResponse, error) {
	payload := map[string]any{
		"channel": channel,
		"text":    text,
	}
	if threadTS != "" {
		payload["thread_ts"] = threadTS
	}
	if blocks != nil {
		payload["blocks"] = blocks
	}
	var out PostMessageResponse
	if err := c.callJSON(ctx, token, "chat.postMessage", payload, &out); err != nil {
		return PostMessageResponse{}, err
	}
	if !out.OK {
		return PostMessageResponse{}, fmt.Errorf("slack chat.postMessage: %s", out.Error)
	}
	return out, nil
}

func (c *Client) UpdateMessage(ctx context.Context, token, channel, ts, text string, blocks any) error {
	payload := map[string]any{
		"channel": channel,
		"ts":      ts,
		"text":    text,
		"blocks":  blocks,
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.callJSON(ctx, token, "chat.update", payload, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("slack chat.update: %s", out.Error)
	}
	return nil
}

func (c *Client) PostEphemeral(ctx context.Context, token, channel, user, text string) error {
	payload := map[string]any{
		"channel": channel,
		"user":    user,
		"text":    text,
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.callJSON(ctx, token, "chat.postEphemeral", payload, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("slack chat.postEphemeral: %s", out.Error)
	}
	return nil
}

func (c *Client) OpenView(ctx context.Context, token, triggerID string, view any) error {
	payload := map[string]any{
		"trigger_id": triggerID,
		"view":       view,
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.callJSON(ctx, token, "views.open", payload, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("slack views.open: %s", out.Error)
	}
	return nil
}

func (c *Client) ExchangeOAuthCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (map[string]any, error) {
	form := url.Values{}
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	form.Set("code", code)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	status, _, body, err := c.doRequestWithRetry(ctx, "oauth.v2.access", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/oauth.v2.access", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	if status >= http.StatusBadRequest {
		return nil, fmt.Errorf("slack oauth.v2.access http %d: %s", status, strings.TrimSpace(string(body)))
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if ok, _ := out["ok"].(bool); !ok {
		return nil, fmt.Errorf("slack oauth error: %v", out["error"])
	}
	return out, nil
}

func (c *Client) ListConversations(ctx context.Context, token string) ([]Conversation, error) {
	cursor := ""
	all := make([]Conversation, 0)
	for {
		query := url.Values{}
		query.Set("exclude_archived", "true")
		query.Set("limit", "1000")
		query.Set("types", "public_channel,private_channel")
		if cursor != "" {
			query.Set("cursor", cursor)
		}

		u := baseURL + "/conversations.list?" + query.Encode()
		status, _, body, err := c.doRequestWithRetry(ctx, "conversations.list", func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		})
		if err != nil {
			return nil, err
		}
		if status >= http.StatusBadRequest {
			return nil, fmt.Errorf("slack conversations.list http %d: %s", status, strings.TrimSpace(string(body)))
		}

		var payload struct {
			OK           bool           `json:"ok"`
			Error        string         `json:"error"`
			Channels     []Conversation `json:"channels"`
			ResponseMeta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}

		if !payload.OK {
			return nil, fmt.Errorf("slack conversations.list: %s", payload.Error)
		}

		all = append(all, payload.Channels...)
		cursor = strings.TrimSpace(payload.ResponseMeta.NextCursor)
		if cursor == "" {
			break
		}
	}

	return all, nil
}

func (c *Client) GetUserEmail(ctx context.Context, token, userID string) (string, error) {
	u := baseURL + "/users.info?user=" + url.QueryEscape(userID)
	status, _, body, err := c.doRequestWithRetry(ctx, "users.info", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	if status >= http.StatusBadRequest {
		return "", fmt.Errorf("slack users.info http %d: %s", status, strings.TrimSpace(string(body)))
	}
	var payload struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			Profile struct {
				Email string `json:"email"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if !payload.OK {
		return "", fmt.Errorf("slack users.info: %s", payload.Error)
	}
	return strings.TrimSpace(payload.User.Profile.Email), nil
}

func (c *Client) ListUsers(ctx context.Context, token string) ([]User, error) {
	cursor := ""
	all := make([]User, 0)
	for {
		query := url.Values{}
		query.Set("limit", "200")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		u := baseURL + "/users.list?" + query.Encode()
		status, _, body, err := c.doRequestWithRetry(ctx, "users.list", func() (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+token)
			return req, nil
		})
		if err != nil {
			return nil, err
		}
		if status >= http.StatusBadRequest {
			return nil, fmt.Errorf("slack users.list http %d: %s", status, strings.TrimSpace(string(body)))
		}

		var payload struct {
			OK      bool   `json:"ok"`
			Error   string `json:"error"`
			Members []struct {
				ID      string `json:"id"`
				Deleted bool   `json:"deleted"`
				IsBot   bool   `json:"is_bot"`
				Profile struct {
					DisplayName string `json:"display_name"`
					RealName    string `json:"real_name"`
					Email       string `json:"email"`
				} `json:"profile"`
			} `json:"members"`
			ResponseMeta struct {
				NextCursor string `json:"next_cursor"`
			} `json:"response_metadata"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, err
		}
		if !payload.OK {
			return nil, fmt.Errorf("slack users.list: %s", payload.Error)
		}
		for _, member := range payload.Members {
			if member.Deleted || member.IsBot || strings.TrimSpace(member.ID) == "" {
				continue
			}
			all = append(all, User{
				ID:          member.ID,
				DisplayName: strings.TrimSpace(member.Profile.DisplayName),
				RealName:    strings.TrimSpace(member.Profile.RealName),
				Email:       strings.TrimSpace(member.Profile.Email),
			})
		}
		cursor = strings.TrimSpace(payload.ResponseMeta.NextCursor)
		if cursor == "" {
			break
		}
	}
	return all, nil
}

func (c *Client) GetUserDisplayName(ctx context.Context, token, userID string) (string, error) {
	id := strings.TrimSpace(userID)
	if id == "" {
		return "", nil
	}

	u := baseURL + "/users.info?user=" + url.QueryEscape(id)
	status, _, body, err := c.doRequestWithRetry(ctx, "users.info", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	if status >= http.StatusBadRequest {
		return "", fmt.Errorf("slack users.info http %d: %s", status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			Name    string `json:"name"`
			Profile struct {
				DisplayName        string `json:"display_name"`
				DisplayNameNorm    string `json:"display_name_normalized"`
				RealName           string `json:"real_name"`
				RealNameNormalized string `json:"real_name_normalized"`
			} `json:"profile"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if !payload.OK {
		return "", fmt.Errorf("slack users.info: %s", payload.Error)
	}

	candidates := []string{
		payload.User.Profile.DisplayName,
		payload.User.Profile.DisplayNameNorm,
		payload.User.Profile.RealName,
		payload.User.Profile.RealNameNormalized,
		payload.User.Name,
	}
	for _, candidate := range candidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			return trimmed, nil
		}
	}

	return id, nil
}

func (c *Client) LookupUserByEmail(ctx context.Context, token, email string) (string, error) {
	normalizedEmail := strings.TrimSpace(email)
	if normalizedEmail == "" {
		return "", nil
	}

	u := baseURL + "/users.lookupByEmail?email=" + url.QueryEscape(normalizedEmail)
	status, _, body, err := c.doRequestWithRetry(ctx, "users.lookupByEmail", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req, nil
	})
	if err != nil {
		return "", err
	}
	if status >= http.StatusBadRequest {
		return "", fmt.Errorf("slack users.lookupByEmail http %d: %s", status, strings.TrimSpace(string(body)))
	}

	var payload struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
		User  struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if !payload.OK {
		if strings.TrimSpace(payload.Error) == "users_not_found" {
			return "", nil
		}
		return "", fmt.Errorf("slack users.lookupByEmail: %s", payload.Error)
	}
	return strings.TrimSpace(payload.User.ID), nil
}

func (c *Client) JoinConversation(ctx context.Context, token, channelID string) error {
	payload := map[string]any{
		"channel": channelID,
	}
	var out struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := c.callJSON(ctx, token, "conversations.join", payload, &out); err != nil {
		return err
	}
	if !out.OK {
		return fmt.Errorf("slack conversations.join: %s", out.Error)
	}
	return nil
}

func (c *Client) callJSON(ctx context.Context, token, method string, payload any, out any) error {
	var lastErr error
	for attempt := 1; attempt <= slackRequestMaxAttempts; attempt++ {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/"+method, buf)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < slackRequestMaxAttempts && shouldRetrySlackNetworkError(err) {
				if sleepErr := sleepWithContext(ctx, nextRetryDelay("", attempt)); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			return err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < slackRequestMaxAttempts {
				if sleepErr := sleepWithContext(ctx, nextRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
					return sleepErr
				}
				continue
			}
			return readErr
		}

		if shouldRetrySlackStatus(resp.StatusCode) && attempt < slackRequestMaxAttempts {
			lastErr = fmt.Errorf("slack %s http %d", method, resp.StatusCode)
			if sleepErr := sleepWithContext(ctx, nextRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return sleepErr
			}
			continue
		}
		if resp.StatusCode >= http.StatusBadRequest {
			return fmt.Errorf("slack %s http %d: %s", method, resp.StatusCode, strings.TrimSpace(string(body)))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return err
		}

		var probe struct {
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(body, &probe); err == nil && !probe.OK && shouldRetrySlackAPIError(probe.Error) && attempt < slackRequestMaxAttempts {
			lastErr = fmt.Errorf("slack %s temporary api error: %s", method, strings.TrimSpace(probe.Error))
			if sleepErr := sleepWithContext(ctx, nextRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return sleepErr
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("slack %s request failed", method)
}

func (c *Client) doRequestWithRetry(ctx context.Context, methodName string, build func() (*http.Request, error)) (int, http.Header, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= slackRequestMaxAttempts; attempt++ {
		req, err := build()
		if err != nil {
			return 0, nil, nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < slackRequestMaxAttempts && shouldRetrySlackNetworkError(err) {
				if sleepErr := sleepWithContext(ctx, nextRetryDelay("", attempt)); sleepErr != nil {
					return 0, nil, nil, sleepErr
				}
				continue
			}
			return 0, nil, nil, err
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			if attempt < slackRequestMaxAttempts {
				if sleepErr := sleepWithContext(ctx, nextRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
					return 0, nil, nil, sleepErr
				}
				continue
			}
			return 0, nil, nil, readErr
		}

		if shouldRetrySlackStatus(resp.StatusCode) && attempt < slackRequestMaxAttempts {
			lastErr = fmt.Errorf("slack %s http %d", methodName, resp.StatusCode)
			if sleepErr := sleepWithContext(ctx, nextRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return 0, nil, nil, sleepErr
			}
			continue
		}
		return resp.StatusCode, resp.Header.Clone(), body, nil
	}

	if lastErr != nil {
		return 0, nil, nil, lastErr
	}
	return 0, nil, nil, fmt.Errorf("slack %s request failed", methodName)
}

func shouldRetrySlackStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func shouldRetrySlackNetworkError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return true
}

func shouldRetrySlackAPIError(errorCode string) bool {
	switch strings.ToLower(strings.TrimSpace(errorCode)) {
	case "ratelimited", "internal_error", "fatal_error", "request_timeout", "service_unavailable":
		return true
	default:
		return false
	}
}

func nextRetryDelay(retryAfterHeader string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfterHeader)); err == nil && secs > 0 {
		delay := time.Duration(secs) * time.Second
		if delay > slackRetryMaxDelay {
			return slackRetryMaxDelay
		}
		return delay
	}
	delay := slackRetryBaseDelay * time.Duration(1<<(attempt-1))
	if delay > slackRetryMaxDelay {
		return slackRetryMaxDelay
	}
	return delay
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
