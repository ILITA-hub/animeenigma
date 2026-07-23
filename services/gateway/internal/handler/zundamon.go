package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const (
	zundamonSpeakerName = "ずんだもん"
	maxZundamonRunes    = 500
	maxAudioQueryBytes  = 2 << 20
	maxSynthesisBytes   = 16 << 20
)

// DegradationLevelReader is the small governor-signal surface required by the
// low-priority TTS facade. Level 1 and above sheds new synthesis work.
type DegradationLevelReader interface {
	Level() int
}

type VoicevoxStyle struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

type voicevoxSpeaker struct {
	Name   string          `json:"name"`
	Styles []VoicevoxStyle `json:"styles"`
}

type zundamonSynthesisRequest struct {
	Text       string  `json:"text"`
	StyleID    int     `json:"styleId"`
	SpeedScale float64 `json:"speedScale"`
	PitchScale float64 `json:"pitchScale"`
}

// ZundamonHandler is a deliberately narrow facade over the internal VOICEVOX
// engine. It exposes only the exact Zundamon speaker, bounds input/output size,
// and serializes CPU-heavy synthesis so the optional feature cannot crowd out
// core traffic.
type ZundamonHandler struct {
	baseURL *url.URL
	client  *http.Client
	level   DegradationLevelReader
	log     *logger.Logger
	slot    chan struct{}

	stylesMu      sync.Mutex
	cachedStyles  []VoicevoxStyle
	stylesExpires time.Time
}

func NewZundamonHandler(target string, level DegradationLevelReader, log *logger.Logger) (*ZundamonHandler, error) {
	baseURL, err := url.Parse(strings.TrimRight(target, "/"))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid VOICEVOX service URL %q", target)
	}
	return &ZundamonHandler{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 45 * time.Second},
		level:   level,
		log:     log,
		slot:    make(chan struct{}, 1),
	}, nil
}

// Status reports engine version plus only the styles belonging to the exact
// official Zundamon speaker. It is shed together with synthesis at Elevated.
func (h *ZundamonHandler) Status(w http.ResponseWriter, r *http.Request) {
	if h.shouldShed() {
		writeZundamonError(w, http.StatusServiceUnavailable, "degraded", "voice synthesis is paused while the platform is under load")
		return
	}

	version, err := h.fetchVersion(r.Context())
	if err != nil {
		h.log.Warnw("VOICEVOX status request failed", "error", err)
		writeZundamonError(w, http.StatusServiceUnavailable, "unavailable", "VOICEVOX is unavailable")
		return
	}
	styles, err := h.fetchZundamonStyles(r.Context(), true)
	if err != nil {
		h.log.Warnw("VOICEVOX speaker discovery failed", "error", err)
		writeZundamonError(w, http.StatusServiceUnavailable, "speaker_missing", "Zundamon is unavailable")
		return
	}

	writeZundamonJSON(w, http.StatusOK, map[string]any{
		"version": version,
		"styles":  styles,
	})
}

// Synthesize creates one bounded WAV response. Only one request is admitted at
// a time; callers receive 429 instead of building an unbounded CPU queue.
func (h *ZundamonHandler) Synthesize(w http.ResponseWriter, r *http.Request) {
	if h.shouldShed() {
		writeZundamonError(w, http.StatusServiceUnavailable, "degraded", "voice synthesis is paused while the platform is under load")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input zundamonSynthesisRequest
	if err := decoder.Decode(&input); err != nil {
		writeZundamonError(w, http.StatusBadRequest, "invalid_request", "invalid synthesis request")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeZundamonError(w, http.StatusBadRequest, "invalid_request", "invalid synthesis request")
		return
	}
	input.Text = strings.TrimSpace(input.Text)
	if input.Text == "" || !utf8.ValidString(input.Text) || utf8.RuneCountInString(input.Text) > maxZundamonRunes {
		writeZundamonError(w, http.StatusBadRequest, "invalid_text", "text must contain 1 to 500 characters")
		return
	}
	if input.SpeedScale < 0.5 || input.SpeedScale > 2 || input.PitchScale < -0.15 || input.PitchScale > 0.15 {
		writeZundamonError(w, http.StatusBadRequest, "invalid_voice_settings", "voice settings are outside the allowed range")
		return
	}

	select {
	case h.slot <- struct{}{}:
		defer func() { <-h.slot }()
	default:
		writeZundamonError(w, http.StatusTooManyRequests, "busy", "Zundamon is already speaking; retry shortly")
		return
	}

	styles, err := h.fetchZundamonStyles(r.Context(), false)
	if err != nil {
		h.log.Warnw("VOICEVOX speaker validation failed", "error", err)
		writeZundamonError(w, http.StatusServiceUnavailable, "unavailable", "VOICEVOX is unavailable")
		return
	}
	if !containsStyle(styles, input.StyleID) {
		writeZundamonError(w, http.StatusBadRequest, "invalid_style", "the selected style does not belong to Zundamon")
		return
	}

	// VOICEVOX can take longer than the gateway's ordinary 30s write deadline
	// during a cold start. The request context and the handler's 45s upstream
	// client timeout remain bounded.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Time{})
	audio, err := h.synthesize(r.Context(), input)
	if err != nil {
		h.log.Warnw("VOICEVOX synthesis failed", "error", err)
		writeZundamonError(w, http.StatusBadGateway, "synthesis_failed", "VOICEVOX could not synthesize this line")
		return
	}

	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(audio)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

func (h *ZundamonHandler) shouldShed() bool {
	return h.level != nil && h.level.Level() >= 1
}

func (h *ZundamonHandler) fetchVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint("/version"), nil)
	if err != nil {
		return "", err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("version returned HTTP %d", resp.StatusCode)
	}
	var version string
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<10)).Decode(&version); err != nil {
		return "", err
	}
	return version, nil
}

func (h *ZundamonHandler) fetchZundamonStyles(ctx context.Context, force bool) ([]VoicevoxStyle, error) {
	h.stylesMu.Lock()
	defer h.stylesMu.Unlock()
	if !force && len(h.cachedStyles) > 0 && time.Now().Before(h.stylesExpires) {
		return append([]VoicevoxStyle(nil), h.cachedStyles...), nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint("/speakers"), nil)
	if err != nil {
		return nil, err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("speakers returned HTTP %d", resp.StatusCode)
	}
	var speakers []voicevoxSpeaker
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&speakers); err != nil {
		return nil, err
	}
	for _, speaker := range speakers {
		if speaker.Name == zundamonSpeakerName && len(speaker.Styles) > 0 {
			h.cachedStyles = append([]VoicevoxStyle(nil), speaker.Styles...)
			h.stylesExpires = time.Now().Add(5 * time.Minute)
			return append([]VoicevoxStyle(nil), speaker.Styles...), nil
		}
	}
	return nil, errors.New("exact Zundamon speaker not found")
}

func (h *ZundamonHandler) synthesize(ctx context.Context, input zundamonSynthesisRequest) ([]byte, error) {
	queryURL, _ := url.Parse(h.endpoint("/audio_query"))
	params := queryURL.Query()
	params.Set("text", input.Text)
	params.Set("speaker", fmt.Sprintf("%d", input.StyleID))
	queryURL.RawQuery = params.Encode()

	queryReq, err := http.NewRequestWithContext(ctx, http.MethodPost, queryURL.String(), nil)
	if err != nil {
		return nil, err
	}
	queryResp, err := h.client.Do(queryReq)
	if err != nil {
		return nil, err
	}
	defer queryResp.Body.Close()
	if queryResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("audio_query returned HTTP %d", queryResp.StatusCode)
	}
	queryBytes, err := io.ReadAll(io.LimitReader(queryResp.Body, maxAudioQueryBytes+1))
	if err != nil || len(queryBytes) > maxAudioQueryBytes {
		return nil, errors.New("audio_query response is invalid or too large")
	}
	var audioQuery map[string]any
	if err := json.Unmarshal(queryBytes, &audioQuery); err != nil {
		return nil, err
	}
	audioQuery["speedScale"] = input.SpeedScale
	audioQuery["pitchScale"] = input.PitchScale
	queryBytes, err = json.Marshal(audioQuery)
	if err != nil {
		return nil, err
	}

	synthesisURL, _ := url.Parse(h.endpoint("/synthesis"))
	params = synthesisURL.Query()
	params.Set("speaker", fmt.Sprintf("%d", input.StyleID))
	synthesisURL.RawQuery = params.Encode()
	synthReq, err := http.NewRequestWithContext(ctx, http.MethodPost, synthesisURL.String(), bytes.NewReader(queryBytes))
	if err != nil {
		return nil, err
	}
	synthReq.Header.Set("Content-Type", "application/json")
	synthReq.Header.Set("Accept", "audio/wav")
	synthResp, err := h.client.Do(synthReq)
	if err != nil {
		return nil, err
	}
	defer synthResp.Body.Close()
	if synthResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("synthesis returned HTTP %d", synthResp.StatusCode)
	}
	audio, err := io.ReadAll(io.LimitReader(synthResp.Body, maxSynthesisBytes+1))
	if err != nil || len(audio) > maxSynthesisBytes {
		return nil, errors.New("synthesis response is invalid or too large")
	}
	return audio, nil
}

func (h *ZundamonHandler) endpoint(path string) string {
	resolved := *h.baseURL
	resolved.Path = strings.TrimRight(resolved.Path, "/") + path
	return resolved.String()
}

func containsStyle(styles []VoicevoxStyle, styleID int) bool {
	for _, style := range styles {
		if style.ID == styleID {
			return true
		}
	}
	return false
}

func writeZundamonError(w http.ResponseWriter, status int, code, message string) {
	writeZundamonJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func writeZundamonJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
