package tasks

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// Scheduler runs a local-time 06:00 reminder loop.
type Scheduler struct {
	service *TaskService
	send    func(context.Context, string) error
	logger  *slog.Logger
	now     func() time.Time
}

func NewScheduler(service *TaskService, send func(context.Context, string) error, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{
		service: service,
		send:    send,
		logger:  logger,
		now:     time.Now,
	}
}

func (s *Scheduler) Run(ctx context.Context) {
	for {
		wait := durationUntilNextSixAM(s.now())
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		if err := s.runTick(ctx); err != nil {
			s.logger.Error("tasks reminder tick failed", "error", err)
		}
	}
}

func (s *Scheduler) runTick(ctx context.Context) error {
	today := s.now().In(time.Local).Format(dateLayout)
	due, err := s.service.PrepareDailyReminder(today)
	if err != nil {
		return fmt.Errorf("select due tasks: %w", err)
	}
	if len(due) == 0 {
		return nil
	}

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := s.send(sendCtx, FormatReminderMessage(today, due)); err != nil {
		return fmt.Errorf("send reminder: %w", err)
	}

	return nil
}

func durationUntilNextSixAM(now time.Time) time.Duration {
	localNow := now.In(time.Local)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 6, 0, 0, 0, localNow.Location())
	if !localNow.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(localNow)
}

func FormatReminderMessage(today string, due []Task) string {
	tasks := make([]Task, len(due))
	copy(tasks, due)
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})

	var b strings.Builder
	fmt.Fprintf(&b, "Tasks for %s\n", today)
	for _, task := range tasks {
		fmt.Fprintf(&b, "%d: %s\n", task.ID, task.Text)
	}
	b.WriteString("Reply /done <id> when finished")
	return b.String()
}
