package core

import "context"

// Receiver polls an external source for inbound messages.
type Receiver interface {
	Start(ctx context.Context) error
}
