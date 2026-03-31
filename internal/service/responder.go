package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gm "github.com/palchrb/inreach-project/internal/hermes"
)

// Responder handles sending response messages back via Hermes API.
type Responder struct {
	api       *gm.HermesAPI
	charLimit int
	logger    *slog.Logger
}

// NewResponder creates a new responder.
func NewResponder(api *gm.HermesAPI, charLimit int, logger *slog.Logger) *Responder {
	return &Responder{api: api, charLimit: charLimit, logger: logger}
}

// Send sends one or more response parts to the sender.
func (r *Responder) Send(ctx context.Context, msg gm.MessageModel, parts []string) error {
	if msg.From == nil {
		return fmt.Errorf("message has no sender")
	}
	to := []string{*msg.From}

	// Flatten and split all parts
	var allParts []string
	for _, part := range parts {
		split := splitMessage(part, r.charLimit)
		allParts = append(allParts, split...)
	}

	// Add part numbers if multiple
	if len(allParts) > 1 {
		for i := range allParts {
			suffix := fmt.Sprintf(" (%d/%d)", i+1, len(allParts))
			if len(allParts[i])+len(suffix) <= r.charLimit {
				allParts[i] += suffix
			}
		}
	}

	for i, part := range allParts {
		r.logger.Info("Sending response", "part", i+1, "total", len(allParts), "length", len(part))
		_, err := r.api.SendMessage(ctx, to, part)
		if err != nil {
			return fmt.Errorf("sending part %d/%d: %w", i+1, len(allParts), err)
		}
		// Short delay between parts to preserve ordering
		if i < len(allParts)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	return nil
}

// splitMessage splits a message on \n boundaries within the char limit.
func splitMessage(message string, maxLength int) []string {
	if len(message) <= maxLength {
		return []string{message}
	}

	var parts []string
	var currentPart string

	lines := strings.Split(message, "\n")
	for _, line := range lines {
		if currentPart == "" {
			currentPart = line
		} else if len(currentPart)+1+len(line) <= maxLength {
			currentPart += "\n" + line
		} else {
			if currentPart != "" {
				parts = append(parts, currentPart)
			}
			currentPart = line
		}

		// Handle lines longer than maxLength
		for len(currentPart) > maxLength {
			parts = append(parts, currentPart[:maxLength])
			currentPart = currentPart[maxLength:]
		}
	}

	if currentPart != "" {
		parts = append(parts, currentPart)
	}

	return parts
}
