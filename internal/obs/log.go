package obs

import (
	"log/slog"
	"os"
)

// NewLogger creates a structured logger with the given level.
func NewLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}

// RoomLogger returns a logger scoped to a specific room.
func RoomLogger(parent *slog.Logger, roomID string) *slog.Logger {
	return parent.With("room", roomID)
}

// ConnLogger returns a logger scoped to a specific connection.
func ConnLogger(parent *slog.Logger, connID uint16) *slog.Logger {
	return parent.With("conn_id", connID)
}
