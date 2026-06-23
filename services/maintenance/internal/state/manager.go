package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/services/maintenance/internal/domain"
)

// Manager handles reading/writing maintenance-state.json and issues.json with atomic writes.
type Manager struct {
	statePath string
	issuePath string
	mu        sync.Mutex
	state     *domain.State
	issues    *domain.IssueDB
}

func NewManager(statePath, issuePath string) *Manager {
	return &Manager{
		statePath: statePath,
		issuePath: issuePath,
	}
}

// Load reads state and issues from disk, creating defaults if missing.
func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load or create state
	s, err := loadJSON[domain.State](m.statePath)
	if err != nil {
		s = &domain.State{
			ActiveAlerts:     make(map[string]domain.ActiveAlert),
			Cooldowns:        make(map[string]string),
			FixAttemptCounts: make(map[string]domain.FixAttemptCount),
			LastFixPerService: make(map[string]domain.LastFix),
			PendingFixes:     make(map[string]domain.PendingFix),
		}
	}
	m.state = s

	// Load or create issues
	db, err := loadJSON[domain.IssueDB](m.issuePath)
	if err != nil {
		db = &domain.IssueDB{
			LastID: 0,
			Issues: []domain.Issue{},
		}
	}
	m.issues = db

	return nil
}

// State returns a deep-copied snapshot of the current state.
//
// *m.state is only a shallow struct copy, so the map fields are reference types
// shared with the Manager. Returning that directly let a caller iterate a map
// (e.g. checkResolvedAlerts ranging ActiveAlerts) concurrently with a
// SetActiveAlert/RemoveActiveAlert write on another goroutine — a fatal
// "concurrent map iteration and map write" panic (audit #34). Deep-copy every
// map so the snapshot is truly independent.
func (m *Manager) State() domain.State {
	m.mu.Lock()
	defer m.mu.Unlock()
	snapshot := *m.state
	snapshot.ActiveAlerts = copyMap(m.state.ActiveAlerts)
	snapshot.Cooldowns = copyMap(m.state.Cooldowns)
	snapshot.FixAttemptCounts = copyMap(m.state.FixAttemptCounts)
	snapshot.LastFixPerService = copyMap(m.state.LastFixPerService)
	snapshot.PendingFixes = copyMap(m.state.PendingFixes)
	return snapshot
}

// copyMap returns a shallow per-entry copy of src (nil-safe). The values are
// flat domain structs, so a value copy is enough to make iteration of the
// returned map safe against concurrent writes to the source map.
func copyMap[K comparable, V any](src map[K]V) map[K]V {
	if src == nil {
		return nil
	}
	dst := make(map[K]V, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// UpdateOffset sets the last processed update ID.
func (m *Manager) UpdateOffset(offset int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.LastUpdateID = offset
	m.state.LastPollAt = nowISO()
}

// SetSessionStarted marks session start time.
func (m *Manager) SetSessionStarted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.SessionStarted = nowISO()
}

// SetBotInfo stores bot identity and reaction support.
func (m *Manager) SetBotInfo(botUserID int64, reactionsSupported bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.BotUserID = botUserID
	m.state.ReactionsSupported = reactionsSupported
}

// --- Active Alerts ---

// GetActiveAlert returns an active alert by key, or nil.
func (m *Manager) GetActiveAlert(key string) *domain.ActiveAlert {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.state.ActiveAlerts[key]; ok {
		return &a
	}
	return nil
}

// SetActiveAlert adds or updates an active alert.
func (m *Manager) SetActiveAlert(key string, alert domain.ActiveAlert) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.ActiveAlerts[key] = alert
}

// RemoveActiveAlert removes an active alert.
func (m *Manager) RemoveActiveAlert(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.ActiveAlerts, key)
}

// CountActiveAlerts returns the number of distinct services with active alerts.
func (m *Manager) CountActiveAlerts() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	services := make(map[string]bool)
	for _, a := range m.state.ActiveAlerts {
		services[a.Service] = true
	}
	return len(services)
}

// --- Cooldowns ---

// IsInCooldown checks if an action:service is in cooldown.
func (m *Manager) IsInCooldown(action, service string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := action + ":" + service
	expiryStr, ok := m.state.Cooldowns[key]
	if !ok {
		return false
	}
	expiry, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil {
		return false
	}
	return time.Now().Before(expiry)
}

// SetCooldown sets a cooldown for action:service.
func (m *Manager) SetCooldown(action, service string, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := action + ":" + service
	m.state.Cooldowns[key] = time.Now().Add(duration).Format(time.RFC3339)
}

// --- Fix Attempts ---

// IncrementFixAttempt increments the fix attempt count and returns the new count.
func (m *Manager) IncrementFixAttempt(alertUID, service string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := alertUID + ":" + service
	attempt, ok := m.state.FixAttemptCounts[key]
	if !ok {
		attempt = domain.FixAttemptCount{FirstAt: nowISO()}
	}
	// Reset if first attempt was >30 min ago
	firstAt, _ := time.Parse(time.RFC3339, attempt.FirstAt)
	if time.Since(firstAt) > 30*time.Minute {
		attempt = domain.FixAttemptCount{FirstAt: nowISO()}
	}
	attempt.Count++
	m.state.FixAttemptCounts[key] = attempt
	return attempt.Count
}

// --- Last Fix Per Service ---

// WasRecentlyFixed checks if a service was fixed within the given duration.
func (m *Manager) WasRecentlyFixed(service string, within time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	fix, ok := m.state.LastFixPerService[service]
	if !ok {
		return false
	}
	fixAt, err := time.Parse(time.RFC3339, fix.At)
	if err != nil {
		return false
	}
	return time.Since(fixAt) < within
}

// RecordFix records that a fix was applied to a service.
func (m *Manager) RecordFix(service, action string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.LastFixPerService[service] = domain.LastFix{
		Action: action,
		At:     nowISO(),
	}
}

// --- Pending Fixes ---

// AddPendingFix stores a pending fix awaiting admin approval.
func (m *Manager) AddPendingFix(issueID string, fix domain.PendingFix) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.PendingFixes[issueID] = fix
}

// GetPendingFix returns a pending fix by issue ID.
func (m *Manager) GetPendingFix(issueID string) *domain.PendingFix {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f, ok := m.state.PendingFixes[issueID]; ok {
		return &f
	}
	return nil
}

// RemovePendingFix removes a pending fix.
func (m *Manager) RemovePendingFix(issueID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.state.PendingFixes, issueID)
}

// ExpirePendingFixes removes pending fixes older than maxAge.
func (m *Manager) ExpirePendingFixes(maxAge time.Duration) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var expired []string
	for id, fix := range m.state.PendingFixes {
		proposedAt, err := time.Parse(time.RFC3339, fix.ProposedAt)
		if err != nil || time.Since(proposedAt) > maxAge {
			expired = append(expired, id)
			delete(m.state.PendingFixes, id)
		}
	}
	return expired
}

// --- Issues ---

// CreateIssue adds a new issue and returns its ID.
func (m *Manager) CreateIssue(issue domain.Issue) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.issues.LastID++
	issue.ID = fmt.Sprintf("AUTO-%03d", m.issues.LastID)
	issue.CreatedAt = nowISO()
	m.issues.Issues = append(m.issues.Issues, issue)
	return issue.ID
}

// UpdateIssue updates an existing issue by ID.
func (m *Manager) UpdateIssue(id string, fn func(*domain.Issue)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.issues.Issues {
		if m.issues.Issues[i].ID == id {
			fn(&m.issues.Issues[i])
			return
		}
	}
}

// FindOpenIssueByAlert finds an open issue for the given service.
//
// Dedup is intentionally by AffectedService only — all alert types for the
// same provider collapse into one open issue to kill duplicate-escalation churn
// (e.g. "stream DOWN" and "latency HIGH" for allanime share a single ticket).
// alertName is accepted for future per-alert-type granularity but is not
// currently used in the match; callers must not assume it narrows the result.
func (m *Manager) FindOpenIssueByAlert(alertName, service string) *domain.Issue {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := len(m.issues.Issues) - 1; i >= 0; i-- {
		issue := &m.issues.Issues[i]
		switch issue.Status {
		case domain.StatusOpen, domain.StatusInvestigating, domain.StatusEscalated:
			if issue.AffectedService == service && issue.Title != "" {
				return issue
			}
		}
	}
	return nil
}

// --- Persistence ---

// Save writes both state and issues to disk atomically.
func (m *Manager) Save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := saveJSON(m.statePath, m.state); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := saveJSON(m.issuePath, m.issues); err != nil {
		return fmt.Errorf("save issues: %w", err)
	}
	return nil
}

// --- Helpers ---

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func loadJSON[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// saveJSON writes JSON atomically using the write-rename pattern.
func saveJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
