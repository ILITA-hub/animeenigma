package domain

import "sync"

type SyncStatus struct {
	mu        sync.RWMutex
	Running   bool   `json:"running"`
	Status    string `json:"status"` // "idle", "syncing", "done", "error"
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	Error     string `json:"error,omitempty"`
	Year      int    `json:"year,omitempty"`
	Season    string `json:"season,omitempty"`
}

func NewSyncStatus() *SyncStatus {
	return &SyncStatus{Status: "idle"}
}

func (s *SyncStatus) Start(year int, season string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = true
	s.Status = "syncing"
	s.Total = 0
	s.Processed = 0
	s.Error = ""
	s.Year = year
	s.Season = season
}

func (s *SyncStatus) SetTotal(total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Total = total
}

func (s *SyncStatus) Increment() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Processed++
}

func (s *SyncStatus) Done() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = false
	s.Status = "done"
}

func (s *SyncStatus) SetError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Running = false
	s.Status = "error"
	s.Error = err
}

func (s *SyncStatus) Get() SyncStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SyncStatus{
		Running:   s.Running,
		Status:    s.Status,
		Total:     s.Total,
		Processed: s.Processed,
		Error:     s.Error,
		Year:      s.Year,
		Season:    s.Season,
	}
}
