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

// Client — минимальный клиент OpenAI.
type Client struct {
	APIKey      string       // токен для Authorization
	AssistantID string       // можно не использовать сейчас, но поле пригодится дальше
	BaseURL     string       // базовый URL API (можно переопределить для тестов/прокси)
	HTTP        *http.Client // общий http-клиент с таймаутом
}

// NewClient — конструктор c разумными дефолтами.
func NewClient(apiKey, assistantID string) *Client {
	return &Client{
		APIKey:      apiKey,
		AssistantID: assistantID,
		BaseURL:     DefaultBaseURL,
		HTTP:        &http.Client{Timeout: DefaultTimeout},
	}
}

// CreateThread — POST /threads → возвращает thread_id.
func (c *Client) CreateThread(ctx context.Context) (string, error) {
	// пустое JSON-тело: OpenAI ожидает объект, даже если без полей
	body := []byte(`{}`)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/threads", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// если OpenAI вернул ошибку — читаем тело и выносим понятную ошибку наверх
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("openai http error: %d %s", resp.StatusCode, string(b))
	}

	// успешный ответ: {"id":"thread_..."}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.ID, nil
}
