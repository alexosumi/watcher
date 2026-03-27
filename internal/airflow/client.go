package airflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const defaultHTTPTimeout = 60 * time.Second

// Client talks to Airflow REST API v1 with HTTP basic auth.
type Client struct {
	baseURL    *url.URL
	user       string
	pass       string
	httpClient *http.Client
	dryRun     bool
}

// PoolState is the subset of GET /api/v1/pools/{name} we need.
type PoolState struct {
	Name            string `json:"name"`
	Slots           int32  `json:"slots"`
	Description     string `json:"description"`
	IncludeDeferred bool   `json:"include_deferred"`
	RunningSlots    *int32 `json:"running_slots,omitempty"`
	ScheduledSlots  *int32 `json:"scheduled_slots,omitempty"`
	QueuedSlots     *int32 `json:"queued_slots,omitempty"`
}

// NewClientFromEnv builds a client using AIRFLOW_HOST, AIRFLOW_USERNAME, AIRFLOW_PASSWORD.
// Trailing slashes on AIRFLOW_HOST are trimmed. DRY_RUN=true skips mutating calls.
func NewClientFromEnv() (*Client, error) {
	host := strings.TrimSpace(os.Getenv("AIRFLOW_HOST"))
	user := strings.TrimSpace(os.Getenv("AIRFLOW_USERNAME"))
	pass := os.Getenv("AIRFLOW_PASSWORD")
	if host == "" || user == "" || pass == "" {
		return nil, fmt.Errorf("AIRFLOW_HOST, AIRFLOW_USERNAME, and AIRFLOW_PASSWORD must be set")
	}
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("parse AIRFLOW_HOST: %w", err)
	}
	if u.Scheme == "" {
		return nil, fmt.Errorf("AIRFLOW_HOST must include a scheme (e.g. https://)")
	}
	u.Path = strings.TrimSuffix(u.Path, "/")

	dry := strings.EqualFold(strings.TrimSpace(os.Getenv("DRY_RUN")), "true")

	return &Client{
		baseURL: u,
		user:    user,
		pass:    pass,
		httpClient: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
		dryRun: dry,
	}, nil
}

func (c *Client) DryRun() bool { return c.dryRun }

// GetPool fetches a pool by name.
func (c *Client) GetPool(poolName string) (*PoolState, error) {
	if poolName == "" {
		return nil, fmt.Errorf("pool name is empty")
	}
	rel := fmt.Sprintf("/api/v1/pools/%s", url.PathEscape(poolName))
	req, err := c.newRequest(http.MethodGet, rel, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("airflow GET %s: %s: %s", rel, resp.Status, truncate(body, 512))
	}
	state, err := decodePoolResponse(body)
	if err != nil {
		return nil, err
	}
	return state, nil
}

func decodePoolResponse(body []byte) (*PoolState, error) {
	var st PoolState
	if err := json.Unmarshal(body, &st); err == nil && st.Name != "" {
		return &st, nil
	}
	var wrap struct {
		Pool PoolState `json:"pool"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return nil, fmt.Errorf("decode pool response: %w", err)
	}
	if wrap.Pool.Name == "" {
		return nil, fmt.Errorf("decode pool response: empty name in wrapped or flat body")
	}
	return &wrap.Pool, nil
}

type patchPoolBody struct {
	Name            string `json:"name"`
	Slots           int32  `json:"slots"`
	Description     string `json:"description"`
	IncludeDeferred bool   `json:"include_deferred"`
}

// PatchPoolSlots updates pool slots while preserving description and include_deferred.
func (c *Client) PatchPoolSlots(poolName string, current *PoolState, newSlots int32) error {
	if c.dryRun {
		return nil
	}
	if poolName == "" {
		return fmt.Errorf("pool name is empty")
	}
	if newSlots < 0 {
		return fmt.Errorf("newSlots must be non-negative")
	}
	body := patchPoolBody{
		Name:            poolName,
		Slots:           newSlots,
		Description:     current.Description,
		IncludeDeferred: current.IncludeDeferred,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	rel := fmt.Sprintf("/api/v1/pools/%s", url.PathEscape(poolName))
	req, err := c.newRequest(http.MethodPatch, rel, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("airflow PATCH %s: %s: %s", rel, resp.Status, truncate(respBody, 512))
	}
	return nil
}

func (c *Client) newRequest(method, rel string, body io.Reader) (*http.Request, error) {
	base := strings.TrimSuffix(c.baseURL.String(), "/")
	if !strings.HasPrefix(rel, "/") {
		rel = "/" + rel
	}
	full := base + rel
	req, err := http.NewRequest(method, full, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.user, c.pass)
	return req, nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
