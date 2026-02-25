package ops_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jdelaire/openslack/core/ops"
	"github.com/jdelaire/openslack/internal/tasks"
)

func newTaskService(t *testing.T) *tasks.TaskService {
	t.Helper()
	store := tasks.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	return tasks.NewTaskService(store).WithClock(func() time.Time {
		return time.Date(2026, 2, 25, 10, 0, 0, 0, time.Local)
	})
}

func TestTaskTomorrowOp(t *testing.T) {
	svc := newTaskService(t)
	op := &ops.TaskTomorrowOp{Service: svc}

	got, err := op.Execute(context.Background(), "Buy eggs")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got != "1: Buy eggs" {
		t.Fatalf("result = %q, want %q", got, "1: Buy eggs")
	}

	got, err = op.Execute(context.Background(), "   ")
	if err != nil {
		t.Fatalf("execute usage: %v", err)
	}
	if got != "Usage: /tomorrow <task description>" {
		t.Fatalf("usage = %q", got)
	}
}

func TestTaskListAndDoneOps(t *testing.T) {
	svc := newTaskService(t)
	tomorrow := &ops.TaskTomorrowOp{Service: svc}
	list := &ops.TaskListOp{Service: svc}
	done := &ops.TaskDoneOp{Service: svc}

	if _, err := tomorrow.Execute(context.Background(), "Task A"); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := tomorrow.Execute(context.Background(), "Task B"); err != nil {
		t.Fatalf("create B: %v", err)
	}

	got, err := list.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := "1: Task A\n2: Task B"
	if got != want {
		t.Fatalf("list = %q, want %q", got, want)
	}

	got, err = done.Execute(context.Background(), "1")
	if err != nil {
		t.Fatalf("done: %v", err)
	}
	if got != "Done: 1" {
		t.Fatalf("done result = %q", got)
	}

	got, err = done.Execute(context.Background(), "1")
	if err != nil {
		t.Fatalf("done twice: %v", err)
	}
	if got != "Already done: 1" {
		t.Fatalf("done twice result = %q", got)
	}

	got, err = done.Execute(context.Background(), "99")
	if err != nil {
		t.Fatalf("done unknown: %v", err)
	}
	if got != "Unknown task: 99" {
		t.Fatalf("done unknown result = %q", got)
	}

	got, err = done.Execute(context.Background(), "one")
	if err != nil {
		t.Fatalf("done usage: %v", err)
	}
	if got != "Usage: /done <id>" {
		t.Fatalf("done usage result = %q", got)
	}

	got, err = list.Execute(context.Background(), "extra")
	if err != nil {
		t.Fatalf("list usage: %v", err)
	}
	if got != "Usage: /tasks" {
		t.Fatalf("list usage result = %q", got)
	}
}
