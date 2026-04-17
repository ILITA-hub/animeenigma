package domain

// GrafanaWebhookPayload matches the Grafana unified alerting webhook contact point JSON.
// Reference: https://grafana.com/docs/grafana/latest/alerting/configure-notifications/manage-contact-points/integrations/webhook-notifier/
type GrafanaWebhookPayload struct {
	Receiver          string                       `json:"receiver"`
	Status            string                       `json:"status"` // "firing" or "resolved"
	Alerts            []GrafanaWebhookAlert        `json:"alerts"`
	GroupLabels       map[string]string             `json:"groupLabels"`
	CommonLabels      map[string]string             `json:"commonLabels"`
	CommonAnnotations map[string]string             `json:"commonAnnotations"`
	ExternalURL       string                       `json:"externalURL"`
	Version           string                       `json:"version"`
	GroupKey          string                       `json:"groupKey"`
	TruncatedAlerts   int                          `json:"truncatedAlerts"`
	OrgID             int                          `json:"orgId"`
	Title             string                       `json:"title"`
	State             string                       `json:"state"`
	Message           string                       `json:"message"`
}

type GrafanaWebhookAlert struct {
	Status       string            `json:"status"` // per-alert status
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"`
	EndsAt       string            `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
	SilenceURL   string            `json:"silenceURL"`
	DashboardURL string            `json:"dashboardURL"`
	PanelURL     string            `json:"panelURL"`
	Values       map[string]any    `json:"values"`
}
