package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/rooms/internal/domain"
)

type RoomService struct {
	cache *cache.RedisCache
	log   *logger.Logger
}

func NewRoomService(cache *cache.RedisCache, log *logger.Logger) *RoomService {
	return &RoomService{
		cache: cache,
		log:   log,
	}
}

// CreateRoom creates a new game room
func (s *RoomService) CreateRoom(ctx context.Context, creatorID string, req *domain.CreateRoomRequest) (*domain.Room, error) {
	room := &domain.Room{
		ID:           generateID(),
		Name:         req.Name,
		CreatorID:    creatorID,
		MaxPlayers:   req.MaxPlayers,
		Status:       "waiting",
		CurrentRound: 0,
		TotalRounds:  10,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Store room in cache
	key := cache.PrefixRoom + room.ID
	if err := s.cache.SetJSON(ctx, key, room, 24*time.Hour); err != nil {
		return nil, fmt.Errorf("store room: %w", err)
	}

	return room, nil
}

// ListRooms returns all available rooms
func (s *RoomService) ListRooms(ctx context.Context) ([]*domain.Room, error) {
	// In a real implementation, this would fetch from Redis or database
	// For now, return empty list
	return []*domain.Room{}, nil
}

// GetRoom returns a specific room
func (s *RoomService) GetRoom(ctx context.Context, roomID string) (*domain.Room, error) {
	var room domain.Room
	key := cache.PrefixRoom + roomID

	err := s.cache.GetJSON(ctx, key, &room)
	if err != nil {
		return nil, errors.NotFound("room")
	}

	return &room, nil
}

// JoinRoom adds a player to a room
func (s *RoomService) JoinRoom(ctx context.Context, roomID, userID, username string) error {
	room, err := s.GetRoom(ctx, roomID)
	if err != nil {
		return err
	}

	if room.Status != "waiting" {
		return errors.InvalidInput("room is not accepting players")
	}

	// Add player to room (simplified)
	player := &domain.Player{
		ID:       generateID(),
		RoomID:   roomID,
		UserID:   userID,
		Username: username,
		Score:    0,
		IsReady:  false,
	}

	// Store player
	key := cache.PrefixRoom + "player:" + player.ID
	if err := s.cache.SetJSON(ctx, key, player, 24*time.Hour); err != nil {
		return fmt.Errorf("store player: %w", err)
	}

	return nil
}

// LeaveRoom removes a player from a room
func (s *RoomService) LeaveRoom(ctx context.Context, roomID, userID string) error {
	// In a real implementation, this would remove the player and clean up
	return nil
}

func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
