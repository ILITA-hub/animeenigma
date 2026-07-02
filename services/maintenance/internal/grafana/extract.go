package grafana

import "strings"

// ExtractService determines the service name from alert labels and annotations.
func ExtractService(labels, annotations map[string]string) string {
	for _, key := range []string{"service", "job", "provider", "player", "group"} {
		if v, ok := labels[key]; ok && v != "" {
			return strings.ToLower(v)
		}
	}
	summary := annotations["summary"]
	serviceNames := []string{"gateway", "auth", "catalog", "streaming", "player", "rooms", "scheduler", "themes", "kodik", "animelib"}
	for _, name := range serviceNames {
		if strings.Contains(strings.ToLower(summary), name) {
			return name
		}
	}
	return "unknown"
}
