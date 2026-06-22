package grafana

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// Client queries Grafana's alertmanager API for current alert states.
type Client struct {
	baseURL string
	user    string
	pass    string
	http    *http.Client
}

// NewClient builds a Grafana alertmanager client. The alertmanager API
// requires authentication; user/pass are sent as HTTP basic auth. When both
// are empty the request is sent unauthenticated (and Grafana will answer 401).
func NewClient(baseURL, user, pass string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		user:    user,
		pass:    pass,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// fetchAlerts GETs the alertmanager alerts with basic auth and decodes the
// JSON array, returning the raw body too. The endpoint requires auth; without
// it Grafana returns a JSON error OBJECT (e.g. 401 {"message":"Unauthorized"}),
// which previously failed with a misleading "cannot unmarshal object into
// []Alert" parse error — so the status code is surfaced explicitly first.
func (c *Client) fetchAlerts() ([]Alert, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/alertmanager/grafana/api/v2/alerts", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("grafana request: %w", err)
	}
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("grafana alerts: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, nil, fmt.Errorf("grafana read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, nil, fmt.Errorf("grafana alerts: unexpected status %d: %s", resp.StatusCode, snippet)
	}

	var alerts []Alert
	if err := json.Unmarshal(body, &alerts); err != nil {
		return nil, nil, fmt.Errorf("grafana parse: %w", err)
	}
	return alerts, body, nil
}

// Alert represents a Grafana alertmanager alert.
type Alert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
	Status      AlertStatus       `json:"status"`
	Fingerprint string            `json:"fingerprint"`
}

type AlertStatus struct {
	State string `json:"state"` // "active", "suppressed", "resolved"
}

// CriticalAlerts are alert names that map to P0.
var CriticalAlerts = map[string]bool{
	"Service Unreachable":  true,
	"Scheduler Sync Stale": true,
	"Player Unavailable":   true,
}

// GetFiringAlerts returns all currently firing (active) alerts from Grafana.
func (c *Client) GetFiringAlerts() ([]domain.ClassifiedMessage, error) {
	alerts, body, err := c.fetchAlerts()
	if err != nil {
		return nil, err
	}

	var result []domain.ClassifiedMessage
	for _, a := range alerts {
		if a.Status.State != "active" {
			continue
		}

		alertName := a.Labels["alertname"]
		service := extractService(a.Labels, a.Annotations)
		severity := "warning"
		priority := domain.P1
		if CriticalAlerts[alertName] {
			severity = "critical"
			priority = domain.P0
		}

		msg := domain.ClassifiedMessage{
			Type:     domain.MessageAlertFiring,
			Priority: priority,
			Text:     fmt.Sprintf("%s: %s", alertName, a.Annotations["summary"]),
			From:     domain.User{Username: "grafana", IsBot: true},
			Alerts: []domain.AlertInfo{{
				Name:        alertName,
				Summary:     a.Annotations["summary"],
				Description: a.Annotations["description"],
				Service:     service,
				Severity:    severity,
			}},
			RawJSON: string(body),
		}
		result = append(result, msg)
	}

	return result, nil
}

// GetAlertFingerprints returns fingerprints of currently active alerts.
// Used to detect when alerts resolve (fingerprint disappears).
func (c *Client) GetAlertFingerprints() (map[string]Alert, error) {
	alerts, _, err := c.fetchAlerts()
	if err != nil {
		return nil, err
	}

	result := make(map[string]Alert)
	for _, a := range alerts {
		key := a.Labels["alertname"] + ":" + extractService(a.Labels, a.Annotations)
		result[key] = a
	}
	return result, nil
}

func extractService(labels, annotations map[string]string) string {
	return ExtractService(labels, annotations)
}
