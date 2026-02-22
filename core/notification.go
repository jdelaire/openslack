package core

import "time"

// Notification represents an outbound notification to be delivered.
type Notification struct {
	ID        string    `json:"id"`
	Text      string    `json:"text"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}
