package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/palchrb/inreach-project/internal/store"
)

// ChatGPTHandler handles general queries via OpenAI API.
type ChatGPTHandler struct {
	apiKey  string
	model   string
	history *store.ChatHistory
}

// NewChatGPTHandler creates a new ChatGPT handler.
func NewChatGPTHandler(apiKey, model string, history *store.ChatHistory) *ChatGPTHandler {
	if model == "" {
		model = "o3-mini"
	}
	return &ChatGPTHandler{apiKey: apiKey, model: model, history: history}
}

func (h *ChatGPTHandler) Name() string { return "chatgpt" }

func (h *ChatGPTHandler) Handle(cc *CommandContext) ([]string, error) {
	if h.apiKey == "" {
		return []string{"ChatGPT API key not configured."}, nil
	}

	convID := cc.Message.ConversationID.String()

	// Get recent conversation context
	recentMsgs := h.history.GetRecentMessages(convID, 5)

	today := time.Now().Format("2006-01-02")
	systemPrompt := fmt.Sprintf("Dagens dato er %s. Du er en generell assistent som hjelper brukere med korte og konsise svar på maks %d tegn og klarer å bygge videre på tidligere kontekst.", today, cc.CharLimit)

	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
	}

	for _, msg := range recentMsgs {
		messages = append(messages, map[string]string{
			"role":    msg.Role,
			"content": msg.Content,
		})
	}

	messages = append(messages, map[string]string{
		"role":    "user",
		"content": cc.Args,
	})

	reply, err := callOpenAI(h.apiKey, h.model, messages)
	if err != nil {
		return nil, fmt.Errorf("ChatGPT API call: %w", err)
	}

	// Truncate if needed
	if len(reply) > cc.CharLimit {
		reply = reply[:cc.CharLimit-3] + "..."
	}

	// Store in history
	h.history.AddMessage(convID, "user", cc.Args)
	h.history.AddMessage(convID, "assistant", reply)

	return []string{reply}, nil
}

// CallOpenAIWithPrompt is a helper for other handlers to call OpenAI with a simple prompt.
func CallOpenAIWithPrompt(apiKey, model, systemPrompt, userPrompt string, maxLen int) (string, error) {
	messages := []map[string]string{
		{"role": "system", "content": systemPrompt},
		{"role": "user", "content": userPrompt},
	}
	reply, err := callOpenAI(apiKey, model, messages)
	if err != nil {
		return "", err
	}
	if len(reply) > maxLen {
		reply = reply[:maxLen-3] + "..."
	}
	return reply, nil
}

func callOpenAI(apiKey, model string, messages []map[string]string) (string, error) {
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	body, _ := json.Marshal(payload)
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("OpenAI API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("parsing OpenAI response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenAI response")
	}

	return result.Choices[0].Message.Content, nil
}
