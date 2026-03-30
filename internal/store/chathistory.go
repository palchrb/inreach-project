package store

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// ChatMessage represents a single message in conversation history.
type ChatMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
}

// ChatHistory stores conversation history keyed by conversation ID.
type ChatHistory struct {
	mu       sync.RWMutex
	messages map[string][]ChatMessage
	filePath string
	ttl      time.Duration
}

// NewChatHistory creates a new chat history store.
func NewChatHistory(filePath string, ttl time.Duration) *ChatHistory {
	ch := &ChatHistory{
		messages: make(map[string][]ChatMessage),
		filePath: filePath,
		ttl:      ttl,
	}
	ch.load()
	return ch
}

// ExcludedPrefixes are command keywords to exclude from ChatGPT context.
var ExcludedPrefixes = []string{
	"vær", "skred", "locate", "shelter", "route", "start rail", "stop rail", "train",
}

// AddMessage adds a message to a conversation's history.
func (ch *ChatHistory) AddMessage(conversationID, role, content string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	ch.messages[conversationID] = append(ch.messages[conversationID], ChatMessage{
		Timestamp: time.Now(),
		Role:      role,
		Content:   content,
	})
	ch.save()
}

// GetRecentMessages returns recent non-command messages for a conversation.
func (ch *ChatHistory) GetRecentMessages(conversationID string, maxPairs int) []ChatMessage {
	ch.mu.RLock()
	defer ch.mu.RUnlock()

	cutoff := time.Now().Add(-ch.ttl)
	var relevant []ChatMessage

	for _, msg := range ch.messages[conversationID] {
		if msg.Timestamp.Before(cutoff) {
			continue
		}
		// Exclude command messages
		lower := strings.ToLower(msg.Content)
		excluded := false
		for _, prefix := range ExcludedPrefixes {
			if strings.HasPrefix(lower, prefix) {
				excluded = true
				break
			}
		}
		if !excluded {
			relevant = append(relevant, msg)
		}
	}

	// Limit to last maxPairs*2 messages
	maxMsgs := maxPairs * 2
	if len(relevant) > maxMsgs {
		relevant = relevant[len(relevant)-maxMsgs:]
	}
	return relevant
}

// Prune removes expired messages from all conversations.
func (ch *ChatHistory) Prune() {
	ch.mu.Lock()
	defer ch.mu.Unlock()

	cutoff := time.Now().Add(-ch.ttl)
	for convID, msgs := range ch.messages {
		var kept []ChatMessage
		for _, m := range msgs {
			if !m.Timestamp.Before(cutoff) {
				kept = append(kept, m)
			}
		}
		if len(kept) == 0 {
			delete(ch.messages, convID)
		} else {
			ch.messages[convID] = kept
		}
	}
	ch.save()
}

func (ch *ChatHistory) load() {
	data, err := os.ReadFile(ch.filePath)
	if err != nil {
		return
	}
	json.Unmarshal(data, &ch.messages)
}

func (ch *ChatHistory) save() {
	if ch.filePath == "" {
		return
	}
	data, _ := json.MarshalIndent(ch.messages, "", "  ")
	os.WriteFile(ch.filePath, data, 0o644)
}
