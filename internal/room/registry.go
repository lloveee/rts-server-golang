package room

import (
	"context"
	"log/slog"
	"rts/internal/lockstep"
	"sync"
)

// Registry manages active rooms.
type Registry struct {
	mu     sync.RWMutex
	rooms  map[string]*Entry
	log    *slog.Logger
	defaults lockstep.RoomConfig
}

// Entry holds a room and its cancel function.
type Entry struct {
	Room   *lockstep.Room
	Cancel context.CancelFunc
}

// NewRegistry creates a room registry.
func NewRegistry(defaults lockstep.RoomConfig, log *slog.Logger) *Registry {
	if log == nil {
		log = slog.Default()
	}
	return &Registry{
		rooms:    make(map[string]*Entry),
		log:      log,
		defaults: defaults,
	}
}

// GetOrCreate returns an existing room or creates a new one.
func (r *Registry) GetOrCreate(roomID string) *lockstep.Room {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.rooms[roomID]; ok {
		return entry.Room
	}

	cfg := r.defaults
	cfg.RoomID = roomID
	cfg.Logger = r.log
	room := lockstep.NewRoom(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	r.rooms[roomID] = &Entry{Room: room, Cancel: cancel}

	go room.Run(ctx)

	r.log.Info("room created", "room_id", roomID)
	return room
}

// Get returns a room by ID, or nil.
func (r *Registry) Get(roomID string) *lockstep.Room {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if entry, ok := r.rooms[roomID]; ok {
		return entry.Room
	}
	return nil
}

// Remove stops and removes a room.
func (r *Registry) Remove(roomID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if entry, ok := r.rooms[roomID]; ok {
		entry.Cancel()
		delete(r.rooms, roomID)
	}
}

// Count returns the number of active rooms.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.rooms)
}
