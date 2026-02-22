package core

import "time"

// InboundMessage represents a message received from Telegram.
type InboundMessage struct {
	UpdateID  int64
	ChatID    int64
	UserID    int64
	Text      string
	Timestamp time.Time
}

// MessageHandler processes an inbound message.
type MessageHandler func(msg InboundMessage)
