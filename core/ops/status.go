package ops

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

var startTime = time.Now()

// StatusOp returns daemon uptime, Go version, and goroutine count.
type StatusOp struct{}

func (s *StatusOp) Name() string        { return "status" }
func (s *StatusOp) Description() string  { return "Show daemon status" }

func (s *StatusOp) Execute(_ context.Context, _ string) (string, error) {
	uptime := time.Since(startTime).Truncate(time.Second)
	return fmt.Sprintf("Status: OK\nUptime: %s\nGo: %s\nGoroutines: %d",
		uptime, runtime.Version(), runtime.NumGoroutine()), nil
}
