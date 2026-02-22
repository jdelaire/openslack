package core

import "context"

// Notifier delivers notifications to an external channel.
type Notifier interface {
	Name() string
	Send(ctx context.Context, n Notification) error
}
