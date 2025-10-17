package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// дефолтные настройки
const DefaultBaseURL = "https://api.openai.com/v1"
const DefaultTimeout = 20 * time.Second
const OpenAIBetaVersion = "assistants=v2"

// Client — минимальный клиент OpenAI.
type Client struct {
	APIKey      string
	AssistantID string
	BaseURL     string
	HTTP        *http.Client
}

// Message представляет сообщение в треде
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   []Content `json:"content"`
	CreatedAt int64     `json:"created_at"`
}

// Content представляет содержимое сообщения
type Content struct {
	Type string `json:"type"`
	Text *Text  `json:"text,omitempty"`
}

// Text представляет текстовое содержимое
type Text struct {
	Value string `json:"value"`
}

// Run представляет запуск ассистента
type Run struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	ThreadID    string `json:"thread_id"`
	AssistantID string `json:"assistant_id"`
}

func NewClient(apiKey, assistantID string) *Client {
	return &Client{
		APIKey:      apiKey,
		AssistantID: assistantID,
		BaseURL:     DefaultBaseURL,
		HTTP:        &http.Client{Timeout: DefaultTimeout},
	}
}

func (c *Client) CreateThread(ctx context.Context) (string, error) {
	body := []byte(`{}`)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/threads", bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", OpenAIBetaVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}

func (c *Client) AddMessage(ctx context.Context, threadID, content string) error {
	payload := map[string]interface{}{
		"role":    "user",
		"content": content,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/threads/%s/messages", c.BaseURL, threadID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", OpenAIBetaVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	return nil
}

func (c *Client) RunAssistant(ctx context.Context, threadID string) (*Run, error) {
	payload := map[string]interface{}{
		"assistant_id": c.AssistantID,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/threads/%s/runs", c.BaseURL, threadID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("OpenAI-Beta", OpenAIBetaVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, err
	}

	return &run, nil
}

func (c *Client) GetRunStatus(ctx context.Context, threadID, runID string) (*Run, error) {
	url := fmt.Sprintf("%s/threads/%s/runs/%s", c.BaseURL, threadID, runID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("OpenAI-Beta", OpenAIBetaVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	var run Run
	if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
		return nil, err
	}

	return &run, nil
}

func (c *Client) WaitForCompletion(ctx context.Context, threadID, runID string, maxWaitTime time.Duration) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	timeout := time.After(maxWaitTime)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for assistant completion")
		case <-ticker.C:
			run, err := c.GetRunStatus(ctx, threadID, runID)
			if err != nil {
				return err
			}

			switch run.Status {
			case "completed":
				return nil
			case "failed", "cancelled", "expired":
				return fmt.Errorf("run failed with status: %s", run.Status)
			case "queued", "in_progress", "cancelling":
				// продолжаем ждать
				continue
			default:
				return fmt.Errorf("unknown run status: %s", run.Status)
			}
		}
	}
}

func (c *Client) GetMessages(ctx context.Context, threadID string, limit int) ([]Message, error) {
	url := fmt.Sprintf("%s/threads/%s/messages?limit=%d&order=desc", c.BaseURL, threadID, limit)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("OpenAI-Beta", OpenAIBetaVersion)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	var result struct {
		Data []Message `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}
