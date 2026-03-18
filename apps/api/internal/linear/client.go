package linear

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

const graphQLEndpoint = "https://api.linear.app/graphql"
const linearRequestMaxAttempts = 3
const linearRetryBaseDelay = 300 * time.Millisecond
const linearRetryMaxDelay = 5 * time.Second

var ErrNotAuthenticated = errors.New("linear not authenticated")

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: 15 * time.Second}}
}

type CreatedIssue struct {
	ID         string
	URL        string
	Identifier string
	Title      string
	AssigneeID *string
	StateID    *string
	StateType  *string
	DueDate    *string
}

type CreateIssueInput struct {
	TeamID      string
	Title       string
	Description string
	AssigneeID  *string
	DueDate     *string
	StateID     *string
}

type Team struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

type WorkflowState struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type UpdateIssueInput struct {
	IssueID     string
	AssigneeID  *string
	DueDate     *string
	StateID     *string
	SetAssignee bool
	SetDueDate  bool
	SetState    bool
}

type IssueSnapshot struct {
	ID            string
	URL           string
	TeamID        string
	StateID       string
	StateType     string
	DueDate       *string
	AssigneeID    *string
	AssigneeEmail *string
}

type OAuthToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
}

func (c *Client) ExchangeOAuthCode(ctx context.Context, clientID, clientSecret, code, redirectURI string) (OAuthToken, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	if redirectURI != "" {
		form.Set("redirect_uri", redirectURI)
	}
	return c.exchangeOAuthToken(ctx, form)
}

func (c *Client) RefreshOAuthToken(ctx context.Context, clientID, clientSecret, refreshToken string) (OAuthToken, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", strings.TrimSpace(refreshToken))
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	return c.exchangeOAuthToken(ctx, form)
}

func (c *Client) exchangeOAuthToken(ctx context.Context, form url.Values) (OAuthToken, error) {
	status, _, body, err := c.doRequestWithRetry(ctx, "oauth.token", func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.linear.app/oauth/token", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return OAuthToken{}, err
	}
	if status >= http.StatusBadRequest {
		return OAuthToken{}, authAwareError("linear oauth failed", strings.TrimSpace(string(body)))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		Error        string `json:"error"`
		Message      string `json:"message"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return OAuthToken{}, err
	}
	if out.AccessToken == "" {
		message := strings.TrimSpace(out.Message)
		if message == "" {
			message = strings.TrimSpace(out.Error)
		}
		if message == "" {
			message = "missing access token"
		}
		return OAuthToken{}, authAwareError("linear oauth failed", message)
	}
	return OAuthToken{
		AccessToken:  out.AccessToken,
		RefreshToken: out.RefreshToken,
		ExpiresIn:    out.ExpiresIn,
	}, nil
}

func (c *Client) CreateIssue(ctx context.Context, token string, input CreateIssueInput) (CreatedIssue, error) {
	if input.TeamID == "" {
		return CreatedIssue{}, fmt.Errorf("linear team id is required")
	}
	q := `mutation IssueCreate($input: IssueCreateInput!) {
  issueCreate(input: $input) {
    success
    issue {
      id
      url
      identifier
      title
      dueDate
      assignee {
        id
      }
      state {
        id
        type
      }
    }
  }
}`
	mutationInput := map[string]any{
		"title":       input.Title,
		"description": input.Description,
		"teamId":      input.TeamID,
	}
	if input.AssigneeID != nil && *input.AssigneeID != "" {
		mutationInput["assigneeId"] = *input.AssigneeID
	}
	if input.DueDate != nil && *input.DueDate != "" {
		mutationInput["dueDate"] = *input.DueDate
	}
	if input.StateID != nil && *input.StateID != "" {
		mutationInput["stateId"] = *input.StateID
	}
	payload := map[string]any{
		"query": q,
		"variables": map[string]any{
			"input": mutationInput,
		},
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return CreatedIssue{}, err
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			IssueCreate struct {
				Success bool `json:"success"`
				Issue   struct {
					ID         string `json:"id"`
					URL        string `json:"url"`
					Identifier string `json:"identifier"`
					Title      string `json:"title"`
					DueDate    string `json:"dueDate"`
					Assignee   *struct {
						ID string `json:"id"`
					} `json:"assignee"`
					State *struct {
						ID   string `json:"id"`
						Type string `json:"type"`
					} `json:"state"`
				} `json:"issue"`
			} `json:"issueCreate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return CreatedIssue{}, err
	}
	if len(out.Errors) > 0 {
		return CreatedIssue{}, authAwareError("linear issue create failed", out.Errors[0].Message)
	}
	if !out.Data.IssueCreate.Success {
		return CreatedIssue{}, fmt.Errorf("linear issue create unsuccessful")
	}
	var assigneeID *string
	var stateID *string
	var stateType *string
	var dueDate *string
	if out.Data.IssueCreate.Issue.Assignee != nil && out.Data.IssueCreate.Issue.Assignee.ID != "" {
		assigneeID = &out.Data.IssueCreate.Issue.Assignee.ID
	}
	if out.Data.IssueCreate.Issue.State != nil {
		if out.Data.IssueCreate.Issue.State.ID != "" {
			stateID = &out.Data.IssueCreate.Issue.State.ID
		}
		if out.Data.IssueCreate.Issue.State.Type != "" {
			stateType = &out.Data.IssueCreate.Issue.State.Type
		}
	}
	if out.Data.IssueCreate.Issue.DueDate != "" {
		dueDate = &out.Data.IssueCreate.Issue.DueDate
	}

	return CreatedIssue{
		ID:         out.Data.IssueCreate.Issue.ID,
		URL:        out.Data.IssueCreate.Issue.URL,
		Identifier: out.Data.IssueCreate.Issue.Identifier,
		Title:      out.Data.IssueCreate.Issue.Title,
		AssigneeID: assigneeID,
		StateID:    stateID,
		StateType:  stateType,
		DueDate:    dueDate,
	}, nil
}

func (c *Client) UpdateIssue(ctx context.Context, token string, input UpdateIssueInput) error {
	if input.IssueID == "" {
		return fmt.Errorf("linear issue id is required")
	}
	update := map[string]any{}
	if input.SetAssignee {
		if input.AssigneeID != nil && *input.AssigneeID != "" {
			update["assigneeId"] = *input.AssigneeID
		} else {
			update["assigneeId"] = nil
		}
	}
	if input.SetDueDate {
		if input.DueDate != nil && *input.DueDate != "" {
			update["dueDate"] = *input.DueDate
		} else {
			update["dueDate"] = nil
		}
	}
	if input.SetState {
		if input.StateID != nil && *input.StateID != "" {
			update["stateId"] = *input.StateID
		} else {
			update["stateId"] = nil
		}
	}
	if len(update) == 0 {
		return nil
	}

	q := `mutation IssueUpdate($id: String!, $input: IssueUpdateInput!) {
  issueUpdate(id: $id, input: $input) {
    success
  }
}`
	payload := map[string]any{
		"query": q,
		"variables": map[string]any{
			"id":    input.IssueID,
			"input": update,
		},
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return err
	}

	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			IssueUpdate struct {
				Success bool `json:"success"`
			} `json:"issueUpdate"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return authAwareError("linear issue update failed", out.Errors[0].Message)
	}
	if !out.Data.IssueUpdate.Success {
		return fmt.Errorf("linear issue update unsuccessful")
	}
	return nil
}

func (c *Client) GetIssue(ctx context.Context, token, issueID string) (IssueSnapshot, error) {
	if strings.TrimSpace(issueID) == "" {
		return IssueSnapshot{}, fmt.Errorf("linear issue id is required")
	}
	q := `query IssueSnapshot($id: String!) {
  issue(id: $id) {
    id
    url
    dueDate
    assignee {
      id
      email
    }
    state {
      id
      type
    }
    team {
      id
    }
  }
}`
	payload := map[string]any{
		"query": q,
		"variables": map[string]any{
			"id": strings.TrimSpace(issueID),
		},
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return IssueSnapshot{}, err
	}

	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Issue *struct {
				ID   string `json:"id"`
				URL  string `json:"url"`
				Team *struct {
					ID string `json:"id"`
				} `json:"team"`
				DueDate  string `json:"dueDate"`
				Assignee *struct {
					ID    string `json:"id"`
					Email string `json:"email"`
				} `json:"assignee"`
				State *struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"state"`
			} `json:"issue"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return IssueSnapshot{}, err
	}
	if len(out.Errors) > 0 {
		return IssueSnapshot{}, authAwareError("linear issue query failed", out.Errors[0].Message)
	}
	if out.Data.Issue == nil || strings.TrimSpace(out.Data.Issue.ID) == "" {
		return IssueSnapshot{}, fmt.Errorf("linear issue not found")
	}

	snapshot := IssueSnapshot{
		ID:  strings.TrimSpace(out.Data.Issue.ID),
		URL: strings.TrimSpace(out.Data.Issue.URL),
	}
	if out.Data.Issue.Team != nil {
		snapshot.TeamID = strings.TrimSpace(out.Data.Issue.Team.ID)
	}
	if out.Data.Issue.State != nil {
		snapshot.StateID = strings.TrimSpace(out.Data.Issue.State.ID)
		snapshot.StateType = strings.TrimSpace(out.Data.Issue.State.Type)
	}
	if due := strings.TrimSpace(out.Data.Issue.DueDate); due != "" {
		snapshot.DueDate = &due
	}
	if out.Data.Issue.Assignee != nil {
		if assigneeID := strings.TrimSpace(out.Data.Issue.Assignee.ID); assigneeID != "" {
			snapshot.AssigneeID = &assigneeID
		}
		if assigneeEmail := strings.TrimSpace(out.Data.Issue.Assignee.Email); assigneeEmail != "" {
			snapshot.AssigneeEmail = &assigneeEmail
		}
	}

	return snapshot, nil
}

func (c *Client) ListTeams(ctx context.Context, token string) ([]Team, error) {
	q := `query Teams {
  teams {
    nodes {
      id
      key
      name
    }
  }
}`
	payload := map[string]any{
		"query": q,
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return nil, err
	}

	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Teams struct {
				Nodes []Team `json:"nodes"`
			} `json:"teams"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, authAwareError("linear teams query failed", out.Errors[0].Message)
	}
	return out.Data.Teams.Nodes, nil
}

func (c *Client) ListTeamStates(ctx context.Context, token, teamID string) ([]WorkflowState, error) {
	if teamID == "" {
		return nil, fmt.Errorf("linear team id is required")
	}
	q := `query TeamStates($teamId: String!) {
  team(id: $teamId) {
    states {
      nodes {
        id
        name
        type
      }
    }
  }
}`
	payload := map[string]any{
		"query": q,
		"variables": map[string]any{
			"teamId": teamID,
		},
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return nil, err
	}

	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Team *struct {
				States struct {
					Nodes []WorkflowState `json:"nodes"`
				} `json:"states"`
			} `json:"team"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, authAwareError("linear states query failed", out.Errors[0].Message)
	}
	if out.Data.Team == nil {
		return nil, nil
	}
	return out.Data.Team.States.Nodes, nil
}

func (c *Client) FindUserIDByEmail(ctx context.Context, token, email string) (string, error) {
	normalizedEmail := strings.TrimSpace(strings.ToLower(email))
	if normalizedEmail == "" {
		return "", nil
	}

	filteredQuery := `query UserByEmail($email: String!) {
  users(filter: { email: { eq: $email } }) {
    nodes {
      id
      email
      active
    }
  }
}`
	filteredPayload := map[string]any{
		"query": filteredQuery,
		"variables": map[string]any{
			"email": normalizedEmail,
		},
	}
	filteredOut, err := c.callGraphQL(ctx, token, filteredPayload)
	if err != nil {
		return "", err
	}
	var filtered struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Users struct {
				Nodes []struct {
					ID     string  `json:"id"`
					Email  *string `json:"email"`
					Active bool    `json:"active"`
				} `json:"nodes"`
			} `json:"users"`
		} `json:"data"`
	}
	if err := json.Unmarshal(filteredOut, &filtered); err == nil && len(filtered.Errors) == 0 {
		for _, user := range filtered.Data.Users.Nodes {
			if user.Email == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(*user.Email)) == normalizedEmail && user.Active {
				return user.ID, nil
			}
		}
		for _, user := range filtered.Data.Users.Nodes {
			if user.Email == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(*user.Email)) == normalizedEmail {
				return user.ID, nil
			}
		}
	} else if len(filtered.Errors) > 0 && isAuthErrorMessage(filtered.Errors[0].Message) {
		return "", authAwareError("linear users query failed", filtered.Errors[0].Message)
	}

	q := `query Users($after: String) {
  users(first: 200, after: $after) {
    nodes {
      id
      email
      active
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`
	cursor := ""
	for {
		payload := map[string]any{
			"query": q,
			"variables": map[string]any{
				"after": nullIfEmpty(cursor),
			},
		}
		raw, err := c.callGraphQL(ctx, token, payload)
		if err != nil {
			return "", err
		}
		var out struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
			Data struct {
				Users struct {
					Nodes []struct {
						ID     string  `json:"id"`
						Email  *string `json:"email"`
						Active bool    `json:"active"`
					} `json:"nodes"`
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
				} `json:"users"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", err
		}
		if len(out.Errors) > 0 {
			return "", authAwareError("linear users query failed", out.Errors[0].Message)
		}
		for _, user := range out.Data.Users.Nodes {
			if user.Email == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(*user.Email)) == normalizedEmail && user.Active {
				return user.ID, nil
			}
		}
		for _, user := range out.Data.Users.Nodes {
			if user.Email == nil {
				continue
			}
			if strings.ToLower(strings.TrimSpace(*user.Email)) == normalizedEmail {
				return user.ID, nil
			}
		}
		if !out.Data.Users.PageInfo.HasNextPage || out.Data.Users.PageInfo.EndCursor == "" {
			break
		}
		cursor = out.Data.Users.PageInfo.EndCursor
	}

	viewerID, viewerEmail, err := c.GetViewer(ctx, token)
	if err != nil {
		return "", err
	}
	if strings.ToLower(strings.TrimSpace(viewerEmail)) == normalizedEmail {
		return viewerID, nil
	}

	return "", nil
}

func (c *Client) GetViewer(ctx context.Context, token string) (string, string, error) {
	q := `query Viewer {
  viewer {
    id
    email
  }
}`
	payload := map[string]any{
		"query": q,
	}
	raw, err := c.callGraphQL(ctx, token, payload)
	if err != nil {
		return "", "", err
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Viewer struct {
				ID    string `json:"id"`
				Email string `json:"email"`
			} `json:"viewer"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", "", err
	}
	if len(out.Errors) > 0 {
		return "", "", authAwareError("linear viewer query failed", out.Errors[0].Message)
	}
	return out.Data.Viewer.ID, out.Data.Viewer.Email, nil
}

func (c *Client) callGraphQL(ctx context.Context, token string, payload map[string]any) ([]byte, error) {
	status, _, out, err := c.doRequestWithRetry(ctx, "graphql", func() (*http.Request, error) {
		buf := &bytes.Buffer{}
		if err := json.NewEncoder(buf).Encode(payload); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, graphQLEndpoint, buf)
		if err != nil {
			return nil, err
		}
		auth := token
		if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			auth = "Bearer " + auth
		}
		req.Header.Set("Authorization", auth)
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	})
	if err != nil {
		return nil, err
	}
	if status >= http.StatusBadRequest {
		message := strings.TrimSpace(string(out))
		if message == "" {
			message = http.StatusText(status)
		}
		err := fmt.Errorf("linear graphql http %d: %s", status, message)
		if status == http.StatusUnauthorized || status == http.StatusForbidden || isAuthErrorMessage(message) {
			return nil, fmt.Errorf("%w: %v", ErrNotAuthenticated, err)
		}
		return nil, err
	}
	return out, nil
}

func (c *Client) doRequestWithRetry(ctx context.Context, methodName string, build func() (*http.Request, error)) (int, http.Header, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= linearRequestMaxAttempts; attempt++ {
		req, err := build()
		if err != nil {
			return 0, nil, nil, err
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if attempt < linearRequestMaxAttempts && shouldRetryLinearNetworkError(err) {
				if sleepErr := sleepWithContext(ctx, nextLinearRetryDelay("", attempt)); sleepErr != nil {
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
			if attempt < linearRequestMaxAttempts {
				if sleepErr := sleepWithContext(ctx, nextLinearRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
					return 0, nil, nil, sleepErr
				}
				continue
			}
			return 0, nil, nil, readErr
		}

		if shouldRetryLinearStatus(resp.StatusCode) && attempt < linearRequestMaxAttempts {
			lastErr = fmt.Errorf("linear %s http %d", methodName, resp.StatusCode)
			if sleepErr := sleepWithContext(ctx, nextLinearRetryDelay(resp.Header.Get("Retry-After"), attempt)); sleepErr != nil {
				return 0, nil, nil, sleepErr
			}
			continue
		}
		return resp.StatusCode, resp.Header.Clone(), body, nil
	}

	if lastErr != nil {
		return 0, nil, nil, lastErr
	}
	return 0, nil, nil, fmt.Errorf("linear %s request failed", methodName)
}

func shouldRetryLinearStatus(status int) bool {
	return status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
}

func shouldRetryLinearNetworkError(err error) bool {
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

func nextLinearRetryDelay(retryAfterHeader string, attempt int) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfterHeader)); err == nil && secs > 0 {
		delay := time.Duration(secs) * time.Second
		if delay > linearRetryMaxDelay {
			return linearRetryMaxDelay
		}
		return delay
	}
	delay := linearRetryBaseDelay * time.Duration(1<<(attempt-1))
	if delay > linearRetryMaxDelay {
		return linearRetryMaxDelay
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

func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotAuthenticated) {
		return true
	}
	return isAuthErrorMessage(err.Error())
}

func authAwareError(prefix, message string) error {
	err := fmt.Errorf("%s: %s", prefix, message)
	if isAuthErrorMessage(message) {
		return fmt.Errorf("%w: %v", ErrNotAuthenticated, err)
	}
	return err
}

func isAuthErrorMessage(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "authentication required") ||
		strings.Contains(lower, "not authenticated") ||
		strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "forbidden") ||
		strings.Contains(lower, "invalid token") ||
		strings.Contains(lower, "expired token") ||
		strings.Contains(lower, "invalid auth")
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}
