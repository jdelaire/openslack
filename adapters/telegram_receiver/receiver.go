package telegram_receiver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jdelaire/openslack/core"
)

const (
	defaultBaseURL  = "https://api.telegram.org"
	longPollTimeout = 30
	httpTimeout     = 35 * time.Second
	errorBackoff    = 5 * time.Second
)

type apiResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result"`
}

type update struct {
	UpdateID int64   `json:"update_id"`
	Message  *message `json:"message"`
}

type message struct {
	MessageID int64  `json:"message_id"`
	From      *user  `json:"from"`
	Chat      chat   `json:"chat"`
	Date      int64  `json:"date"`
	Text      string `json:"text"`
}

type user struct {
	ID int64 `json:"id"`
}

type chat struct {
	ID int64 `json:"id"`
}

// Receiver long-polls Telegram for inbound messages.
type Receiver struct {
	botToken string
	handler  core.MessageHandler
	logger   *slog.Logger
	client   *http.Client
	baseURL  string
	offset   int64
}

// New creates a Telegram receiver.
func New(botToken string, handler core.MessageHandler, logger *slog.Logger) *Receiver {
	return &Receiver{
		botToken: botToken,
		handler:  handler,
		logger:   logger,
		client:   &http.Client{Timeout: httpTimeout},
		baseURL:  defaultBaseURL,
	}
}

// WithBaseURL overrides the Telegram API base URL (for testing).
func (r *Receiver) WithBaseURL(url string) *Receiver {
	r.baseURL = url
	return r
}

// Start begins the long-poll loop. Blocks until ctx is cancelled.
func (r *Receiver) Start(ctx context.Context) error {
	r.logger.Info("telegram receiver started")
	for {
		if err := ctx.Err(); err != nil {
			r.logger.Info("telegram receiver stopped")
			return nil
		}

		updates, err := r.poll(ctx)
		if err != nil {
			if ctx.Err() != nil {
				r.logger.Info("telegram receiver stopped")
				return nil
			}
			r.logger.Error("poll error", "error", err)
			select {
			case <-time.After(errorBackoff):
			case <-ctx.Done():
				return nil
			}
			continue
		}

		for _, u := range updates {
			if u.Message == nil || u.Message.Text == "" {
				r.offset = u.UpdateID + 1
				continue
			}

			var userID int64
			if u.Message.From != nil {
				userID = u.Message.From.ID
			}

			msg := core.InboundMessage{
				UpdateID:  u.UpdateID,
				ChatID:    u.Message.Chat.ID,
				UserID:    userID,
				Text:      u.Message.Text,
				Timestamp: time.Unix(u.Message.Date, 0),
			}

			r.handler(msg)
			r.offset = u.UpdateID + 1
		}
	}
}

func (r *Receiver) poll(ctx context.Context) ([]update, error) {
	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=%d",
		r.baseURL, r.botToken, r.offset, longPollTimeout)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api status: %d", resp.StatusCode)
	}

	var apiResp apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("api returned ok=false")
	}

	var updates []update
	if err := json.Unmarshal(apiResp.Result, &updates); err != nil {
		return nil, fmt.Errorf("decode updates: %w", err)
	}

	return updates, nil
}
