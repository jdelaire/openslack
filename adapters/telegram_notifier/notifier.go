package telegram_notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jdelaire/openslack/core"
)

// Notifier sends notifications via the Telegram Bot API.
type Notifier struct {
	botToken string
	chatID   string
	client   *http.Client
	baseURL  string
}

// New creates a Telegram notifier with the given bot token and chat ID.
func New(botToken, chatID string) *Notifier {
	return &Notifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
		baseURL:  "https://api.telegram.org",
	}
}

func (n *Notifier) Name() string { return "telegram" }

func (n *Notifier) Send(ctx context.Context, notif core.Notification) error {
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", n.baseURL, n.botToken)

	resp, err := n.client.PostForm(endpoint, url.Values{
		"chat_id": {n.chatID},
		"text":    {notif.Text},
	})
	if err != nil {
		return fmt.Errorf("telegram request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var body struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		json.NewDecoder(resp.Body).Decode(&body)
		return fmt.Errorf("telegram API error %d: %s", resp.StatusCode, body.Description)
	}

	return nil
}

// WithBaseURL sets a custom base URL (for testing).
func (n *Notifier) WithBaseURL(baseURL string) *Notifier {
	n.baseURL = baseURL
	return n
}
