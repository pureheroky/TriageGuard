package stripe

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBaseURL = "https://api.stripe.com/v1"

type Client struct {
	secretKey string
	http      *http.Client
}

type Subscription struct {
	ID                string
	CustomerID        string
	Status            string
	CancelAtPeriodEnd bool
	CurrentPeriodEnd  *time.Time
	PriceID           string
}

func NewClient(secretKey string) *Client {
	return &Client{
		secretKey: strings.TrimSpace(secretKey),
		http: &http.Client{
			Timeout: 12 * time.Second,
		},
	}
}

func (c *Client) Enabled() bool {
	return c != nil && strings.TrimSpace(c.secretKey) != ""
}

func (c *Client) CreateBillingPortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	if !c.Enabled() {
		return "", fmt.Errorf("stripe is not configured")
	}
	values := url.Values{}
	values.Set("customer", strings.TrimSpace(customerID))
	values.Set("return_url", strings.TrimSpace(returnURL))

	var resp struct {
		URL string `json:"url"`
	}
	if err := c.doForm(ctx, http.MethodPost, "/billing_portal/sessions", values, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.URL) == "" {
		return "", fmt.Errorf("stripe billing portal url is empty")
	}
	return resp.URL, nil
}

func (c *Client) GetSubscription(ctx context.Context, subscriptionID string) (Subscription, error) {
	if !c.Enabled() {
		return Subscription{}, fmt.Errorf("stripe is not configured")
	}
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return Subscription{}, fmt.Errorf("stripe subscription id is required")
	}

	var raw struct {
		ID                string `json:"id"`
		Customer          string `json:"customer"`
		Status            string `json:"status"`
		CancelAtPeriodEnd bool   `json:"cancel_at_period_end"`
		CurrentPeriodEnd  int64  `json:"current_period_end"`
		Items             struct {
			Data []struct {
				Price struct {
					ID string `json:"id"`
				} `json:"price"`
			} `json:"data"`
		} `json:"items"`
	}
	if err := c.doForm(ctx, http.MethodGet, "/subscriptions/"+url.PathEscape(subscriptionID), nil, &raw); err != nil {
		return Subscription{}, err
	}

	sub := Subscription{
		ID:                strings.TrimSpace(raw.ID),
		CustomerID:        strings.TrimSpace(raw.Customer),
		Status:            strings.TrimSpace(raw.Status),
		CancelAtPeriodEnd: raw.CancelAtPeriodEnd,
	}
	if raw.CurrentPeriodEnd > 0 {
		t := time.Unix(raw.CurrentPeriodEnd, 0).UTC()
		sub.CurrentPeriodEnd = &t
	}
	if len(raw.Items.Data) > 0 {
		sub.PriceID = strings.TrimSpace(raw.Items.Data[0].Price.ID)
	}
	return sub, nil
}

func (c *Client) doForm(ctx context.Context, method, path string, values url.Values, out any) error {
	fullURL := strings.TrimSuffix(apiBaseURL, "/") + path
	var body io.Reader
	if method == http.MethodGet {
		if values != nil && len(values) > 0 {
			fullURL += "?" + values.Encode()
		}
	} else if values != nil {
		body = strings.NewReader(values.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, body)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.secretKey, "")
	if method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("stripe api %s %s failed: status=%d body=%s", method, path, resp.StatusCode, truncate(string(respBody), 280))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("stripe response parse error: %w", err)
	}
	return nil
}

func truncate(v string, limit int) string {
	if limit <= 0 || len(v) <= limit {
		return v
	}
	return v[:limit]
}

func ParseStripeSignatureHeader(raw string) (timestamp int64, signatures []string, err error) {
	parts := strings.Split(strings.TrimSpace(raw), ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			parsed, parseErr := strconv.ParseInt(kv[1], 10, 64)
			if parseErr != nil {
				return 0, nil, parseErr
			}
			timestamp = parsed
		case "v1":
			if strings.TrimSpace(kv[1]) != "" {
				signatures = append(signatures, strings.TrimSpace(kv[1]))
			}
		}
	}
	if timestamp <= 0 {
		return 0, nil, fmt.Errorf("missing stripe signature timestamp")
	}
	if len(signatures) == 0 {
		return 0, nil, fmt.Errorf("missing stripe v1 signatures")
	}
	return timestamp, signatures, nil
}
