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
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
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

// criticalAlerts are alert names that map to P0.
var criticalAlerts = map[string]bool{
	"Service Unreachable":  true,
	"Scheduler Sync Stale": true,
	"Player Unavailable":   true,
}

// GetFiringAlerts returns all currently firing (active) alerts from Grafana.
func (c *Client) GetFiringAlerts() ([]domain.ClassifiedMessage, error) {
	url := c.baseURL + "/api/alertmanager/grafana/api/v2/alerts"
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("grafana alerts: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("grafana read: %w", err)
	}

	var alerts []Alert
	if err := json.Unmarshal(body, &alerts); err != nil {
		return nil, fmt.Errorf("grafana parse: %w", err)
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
		if criticalAlerts[alertName] {
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
	url := c.baseURL + "/api/alertmanager/grafana/api/v2/alerts"
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var alerts []Alert
	if err := json.Unmarshal(body, &alerts); err != nil {
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
	// Check common label fields
	for _, key := range []string{"service", "job", "provider", "player"} {
		if v, ok := labels[key]; ok && v != "" {
			return strings.ToLower(v)
		}
	}
	// Check annotations summary for service names
	summary := annotations["summary"]
	serviceNames := []string{"gateway", "auth", "catalog", "streaming", "player", "rooms", "scheduler", "themes", "kodik", "animelib", "hianime", "consumet", "aniwatch"}
	for _, name := range serviceNames {
		if strings.Contains(strings.ToLower(summary), name) {
			return name
		}
	}
	return "unknown"
}
