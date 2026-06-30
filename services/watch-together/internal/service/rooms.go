package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/watch-together/internal/repo"
)

// snapshotMessageLimit is the cap for messages returned by Get → RoomSnapshot.
// Redis stores up to 100 chat messages (LTRIM 0 99 in the repo); the
// snapshot returns the most recent 50 to new joiners per the design doc
// §room:snapshot.
const snapshotMessageLimit = 50

// Sentinel errors returned by RoomService. Handlers map these onto HTTP
// status codes via errors.Is — never via type assertion against
// libs/errors.AppError, because we want service callers to be able to
// treat these as opaque sentinels regardless of underlying transport.
//
// ErrInvalidInput → 400 BadRequest at handler layer
// ErrNotHost      → 403 Forbidden  at handler layer
// repo.ErrNotFound → 410 Gone      at handler layer (re-exported via service.ErrNotFound for caller convenience)
var (
	// ErrInvalidInput signals Create-time validation failure (empty field,
	// unknown player constant). Handler should reply 400 with err.Error()
	// (safe — message is service-author-controlled, no user data leak).
	ErrInvalidInput = stderrors.New("watch-together: invalid input")

	// ErrNotHost signals a Delete attempt by a user_id that does not match
	// the stored host_user_id (WT-FOUND-03 — host-only force-close).
	ErrNotHost = stderrors.New("watch-together: not host")

	// ErrNotFound is a thin re-export of repo.ErrNotFound so handler code
	// can `errors.Is(err, service.ErrNotFound)` without importing the repo
	// package directly. Same underlying sentinel — errors.Is(repo.ErrNotFound, service.ErrNotFound)
	// is true.
	ErrNotFound = repo.ErrNotFound
)

// allowedPlayers is the set of player identifiers the room may declare.
// Mirrors the frontend player components (CLAUDE.md §Video Player
// Architecture) and the constants in domain/ws_message.go.
var allowedPlayers = map[string]struct{}{
	domain.PlayerKodik:      {},
	domain.PlayerAnimeLib:   {},
	domain.PlayerOurEnglish: {},
	domain.PlayerHanime:     {},
	domain.PlayerAePlayer:   {},
}

// CreateRoomInput is the transport-agnostic payload for RoomService.Create.
// Handler code decodes JSON into this directly; tests construct it inline.
type CreateRoomInput struct {
	AnimeID       string
	EpisodeID     string
	Player        string
	TranslationID string
}

// validate returns ErrInvalidInput-wrapped detail on missing/unknown fields.
// Player is checked against the allowedPlayers set so a typo (e.g. "vlc")
// rejects at the service layer rather than landing as garbage in Redis.
func (in CreateRoomInput) validate() error {
	if in.AnimeID == "" {
		return fmt.Errorf("%w: anime_id is required", ErrInvalidInput)
	}
	if in.EpisodeID == "" {
		return fmt.Errorf("%w: episode_id is required", ErrInvalidInput)
	}
	if in.Player == "" {
		return fmt.Errorf("%w: player is required", ErrInvalidInput)
	}
	if _, ok := allowedPlayers[in.Player]; !ok {
		return fmt.Errorf("%w: unknown player %q (allowed: kodik|animelib|ourenglish|hanime|aeplayer)", ErrInvalidInput, in.Player)
	}
	// translation_id is required for the legacy single-source players (it
	// names the exact upstream stream). The first-party aePlayer carries its
	// source selection as an opaque combo token and resolves a smart default
	// when the token is empty, so aeplayer rooms may be created token-less —
	// the host's player picks the BEST source and broadcasts it to joiners.
	if in.TranslationID == "" && in.Player != domain.PlayerAePlayer {
		return fmt.Errorf("%w: translation_id is required", ErrInvalidInput)
	}
	return nil
}

// RoomService is the single mutation surface for room lifecycle. Handlers
// MUST NOT call repo.RoomRepo methods directly — every state change goes
// through one of (Create, Get, Delete) so validation + metric bumps + audit
// logging stay co-located.
//
// newID / now are injection points so tests can drive deterministic UUIDs
// and timestamps; production callers use the defaults wired in
// NewRoomService.
type RoomService struct {
	repo *repo.RoomRepo
	log  *logger.Logger

	// newID returns the UUID assigned to a newly-created room. Defaults to
	// uuid.NewString. Tests override to assert exact room_id values.
	newID func() string
	// now returns the current time. Defaults to time.Now. Tests override to
	// pin CreatedAt / PlaybackTimeUpdatedAtMs to a known instant so equality
	// assertions are stable.
	now func() time.Time
}

// NewRoomService wires the repo + logger and installs the default uuid +
// time providers. Pass nil for log to fall back to logger.Default().
func NewRoomService(r *repo.RoomRepo, log *logger.Logger) *RoomService {
	if log == nil {
		log = logger.Default()
	}
	return &RoomService{
		repo:  r,
		log:   log,
		newID: uuid.NewString,
		now:   time.Now,
	}
}

// Create allocates a fresh room HASH in Redis with `hostUserID` recorded as
// the cosmetic host (WT-FOUND-03 — only the host can call Delete). Returns
// the persisted Room on success; the handler builds the API response from
// these fields (room_id = Room.ID, invite_url / ws_url constructed against
// cfg.PublicBaseURL).
//
// On success bumps wt_room_create_total. Validation failures DO NOT bump
// the counter (otherwise a bug-spam loop would flood the metric).
//
// hostUsername is currently unused at the service layer — the host's chat
// display name comes from JWT claims when they later connect on the
// WebSocket in 01.5. We accept it now so the handler signature is stable
// once 01.5 adds member-meta plumbing.
func (s *RoomService) Create(ctx context.Context, hostUserID, hostUsername string, in CreateRoomInput) (*domain.Room, error) {
	if hostUserID == "" {
		return nil, fmt.Errorf("%w: host user_id is required", ErrInvalidInput)
	}
	if err := in.validate(); err != nil {
		return nil, err
	}

	now := s.now()
	room := &domain.Room{
		ID:                      s.newID(),
		CreatedAt:               now.Unix(),
		AnimeID:                 in.AnimeID,
		EpisodeID:               in.EpisodeID,
		Player:                  in.Player,
		TranslationID:           in.TranslationID,
		PlaybackState:           domain.StatePaused,
		PlaybackTime:            0,
		PlaybackTimeUpdatedAtMs: now.UnixMilli(),
		HostUserID:              hostUserID,
	}

	if err := s.repo.CreateRoom(ctx, room); err != nil {
		return nil, err
	}

	// Metric bump only after successful persistence — validation failures
	// (caught above) do not contribute to the counter.
	RoomCreateTotal.Inc()
	// Plan 05.2 WT-NF-06 — live gauge for the Grafana panel. Inverse Dec
	// lives in observeRoomTeardown (called from Delete + grace.fire).
	RoomsActive.Inc()

	s.log.Infow("watch_together create room",
		"room_id", room.ID,
		"host_user_id", hostUserID,
		"anime_id", in.AnimeID,
		"episode_id", in.EpisodeID,
		"player", in.Player,
	)
	_ = hostUsername // reserved for 01.5 member-meta wiring; keep param shape stable
	return room, nil
}

// Get assembles a full RoomSnapshot for the freshly-connected client (and
// for GET /api/watch-together/rooms/{id}). Returns ErrNotFound (== repo.ErrNotFound)
// if the room HASH is gone — either expired by TTL or explicitly deleted.
//
// Members come from the live Redis HASH so a snapshot fetched mid-call
// reflects whoever is currently registered there. Messages are returned
// in CHRONOLOGICAL order (oldest-first) — the repo flips Redis's
// newest-at-head LPUSH layout for us.
func (s *RoomService) Get(ctx context.Context, roomID string) (*domain.RoomSnapshot, error) {
	if roomID == "" {
		return nil, fmt.Errorf("%w: room_id is required", ErrInvalidInput)
	}

	room, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		return nil, err
	}

	members, err := s.repo.ListMembers(ctx, roomID)
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []domain.Member{}
	}

	messages, err := s.repo.GetMessages(ctx, roomID, snapshotMessageLimit)
	if err != nil {
		return nil, err
	}
	if messages == nil {
		messages = []domain.ChatMessage{}
	}

	return &domain.RoomSnapshot{
		Room:            *room,
		Members:         members,
		Messages:        messages,
		ProtocolVersion: domain.ProtocolVersion,
	}, nil
}

// Delete tears down the room — but ONLY if requesterUserID matches the
// stored host_user_id (WT-FOUND-03 — host-only force-close). Any other
// caller gets ErrNotHost and the room is untouched.
//
// On success this removes wt:room:{id}, wt:room:{id}:members, and
// wt:room:{id}:messages atomically. Connected members are kicked by the
// handler layer (handler/rooms.go, Plan 05.1), which broadcasts `room:closed`
// to members and cancels the grace timer BEFORE invoking this delete — so this
// method only owns the Redis teardown.
func (s *RoomService) Delete(ctx context.Context, requesterUserID, roomID string) error {
	if requesterUserID == "" {
		return fmt.Errorf("%w: requester user_id is required", ErrInvalidInput)
	}
	if roomID == "" {
		return fmt.Errorf("%w: room_id is required", ErrInvalidInput)
	}

	room, err := s.repo.GetRoom(ctx, roomID)
	if err != nil {
		// Includes ErrNotFound — propagate so handler can map → 410 Gone.
		return err
	}

	if room.HostUserID != requesterUserID {
		s.log.Infow("watch_together delete denied non-host",
			"room_id", roomID,
			"host_user_id", room.HostUserID,
			"requester_user_id", requesterUserID,
		)
		return ErrNotHost
	}

	// Plan 05.2 WT-NF-06 — observe session duration + chat count + Dec the
	// active gauge BEFORE the destructive DeleteRoom call. Best-effort:
	// observation failures must not block the delete.
	observeRoomTeardown(ctx, s.repo, s.log, room)

	if err := s.repo.DeleteRoom(ctx, roomID); err != nil {
		return err
	}

	s.log.Infow("watch_together delete room",
		"room_id", roomID,
		"host_user_id", room.HostUserID,
	)
	return nil
}

// observeRoomTeardown is the Plan 05.2 telemetry helper shared between
// RoomService.Delete and GraceManager.fire (05.1's territory — 05.1 may
// wire this call into grace.go after this plan lands). It:
//
//  1. Observes wt_session_duration_seconds = wall-clock since room.CreatedAt.
//  2. Observes wt_chat_messages_per_room = LLEN on the chat list (via
//     repo.MessageCount). Zero is a valid observation (test rooms with
//     no chat traffic still observe — the 0 bucket is meaningful).
//  3. Decrements wt_rooms_active.
//
// All three observations are best-effort: a MessageCount error logs and
// observes 0 (rather than blocking the teardown) because telemetry
// failures must never prevent the actual room deletion. RoomsActive.Dec
// runs unconditionally — the room exists at this call site, so the gauge
// MUST move down to keep the live-rooms count honest.
//
// Pass a non-nil *domain.Room — the caller is expected to have already
// fetched it via repo.GetRoom (skipping observation entirely if the room
// vanished mid-teardown).
func observeRoomTeardown(ctx context.Context, r *repo.RoomRepo, log *logger.Logger, room *domain.Room) {
	if room == nil {
		// Defensive: caller should have early-returned on a missing room.
		// If we got here with nil, just decrement the gauge (room must
		// have existed at Inc time) and skip the observations.
		RoomsActive.Dec()
		return
	}

	// CreatedAt is unix seconds (domain/room.go). time.Since wants a
	// time.Time, so convert. If the clock went backwards (negative
	// elapsed), Observe a 0 rather than a negative — histograms accept
	// negatives but the bucket layout starts at 60s anyway, so a
	// negative would just sit in the lowest bucket and skew the
	// distribution.
	createdAt := time.Unix(room.CreatedAt, 0)
	elapsedSec := time.Since(createdAt).Seconds()
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	SessionDurationSeconds.Observe(elapsedSec)

	n, err := r.MessageCount(ctx, room.ID)
	if err != nil {
		log.Debugw("watch_together teardown observe message_count failed",
			"room_id", room.ID,
			"err", err,
		)
		n = 0
	}
	ChatMessagesPerRoom.Observe(float64(n))

	RoomsActive.Dec()
}
