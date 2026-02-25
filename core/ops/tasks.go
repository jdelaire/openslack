package ops

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tasksvc "github.com/jdelaire/openslack/internal/tasks"
)

// TaskTomorrowOp creates a task that starts tomorrow.
type TaskTomorrowOp struct {
	Service *tasksvc.TaskService
}

func (o *TaskTomorrowOp) Name() string        { return "tomorrow" }
func (o *TaskTomorrowOp) Description() string { return "Create a task that starts tomorrow" }
func (o *TaskTomorrowOp) Risk() RiskLevel     { return RiskNone }

func (o *TaskTomorrowOp) Execute(_ context.Context, args string) (string, error) {
	task, err := o.Service.CreateTomorrow(args)
	if err != nil {
		if errors.Is(err, tasksvc.ErrEmptyTaskText) {
			return "Usage: /tomorrow <task description>", nil
		}
		return "", err
	}
	return fmt.Sprintf("%d: %s", task.ID, task.Text), nil
}

// TaskListOp lists all open tasks.
type TaskListOp struct {
	Service *tasksvc.TaskService
}

func (o *TaskListOp) Name() string        { return "tasks" }
func (o *TaskListOp) Description() string { return "List open tasks" }
func (o *TaskListOp) Risk() RiskLevel     { return RiskNone }

func (o *TaskListOp) Execute(_ context.Context, args string) (string, error) {
	if strings.TrimSpace(args) != "" {
		return "Usage: /tasks", nil
	}

	tasks, err := o.Service.ListOpen()
	if err != nil {
		return "", err
	}
	if len(tasks) == 0 {
		return "No open tasks.", nil
	}

	lines := make([]string, 0, len(tasks))
	for _, task := range tasks {
		lines = append(lines, fmt.Sprintf("%d: %s", task.ID, task.Text))
	}
	return strings.Join(lines, "\n"), nil
}

// TaskDoneOp marks a task done.
type TaskDoneOp struct {
	Service *tasksvc.TaskService
}

func (o *TaskDoneOp) Name() string        { return "done" }
func (o *TaskDoneOp) Description() string { return "Mark a task as done" }
func (o *TaskDoneOp) Risk() RiskLevel     { return RiskNone }

func (o *TaskDoneOp) Execute(_ context.Context, args string) (string, error) {
	id, ok := parseDoneID(args)
	if !ok {
		return "Usage: /done <id>", nil
	}

	status, err := o.Service.Complete(id)
	if err != nil {
		return "", err
	}

	switch status {
	case tasksvc.CompleteUpdated:
		return fmt.Sprintf("Done: %d", id), nil
	case tasksvc.CompleteAlreadyDone:
		return fmt.Sprintf("Already done: %d", id), nil
	default:
		return fmt.Sprintf("Unknown task: %d", id), nil
	}
}

func parseDoneID(args string) (int, bool) {
	parts := strings.Fields(strings.TrimSpace(args))
	if len(parts) != 1 {
		return 0, false
	}
	id, err := strconv.Atoi(parts[0])
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
