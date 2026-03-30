package command

import (
	"context"
	"log/slog"

	gm "github.com/palchrb/inreach-project/internal/hermes"
)

// CommandContext carries everything a handler needs.
type CommandContext struct {
	Ctx       context.Context
	Message   gm.MessageModel
	Args      string   // Message body with command prefix stripped
	Lat       *float64 // From UserLocation
	Lon       *float64
	Elevation *float64
	CharLimit int
	Logger    *slog.Logger
}

// Handler processes a command and returns response text parts.
type Handler interface {
	Name() string
	Handle(cc *CommandContext) ([]string, error)
}
