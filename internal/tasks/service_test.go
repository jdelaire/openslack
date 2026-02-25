package tasks_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/jdelaire/openslack/internal/tasks"
)

func TestCreateTomorrowIncrementsID(t *testing.T) {
	store := tasks.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	svc := tasks.NewTaskService(store).WithClock(func() time.Time {
		return time.Date(2026, 2, 25, 21, 30, 0, 0, time.Local)
	})

	first, err := svc.CreateTomorrow("Buy eggs")
	if err != nil {
		t.Fatalf("create first task: %v", err)
	}
	second, err := svc.CreateTomorrow("Call landlord")
	if err != nil {
		t.Fatalf("create second task: %v", err)
	}

	if first.ID != 1 || second.ID != 2 {
		t.Fatalf("ids = %d,%d; want 1,2", first.ID, second.ID)
	}
	if first.StartDate != "2026-02-26" || second.StartDate != "2026-02-26" {
		t.Fatalf("unexpected start dates: %q, %q", first.StartDate, second.StartDate)
	}
	if first.Schedule != "daily_6am" || second.Schedule != "daily_6am" {
		t.Fatalf("unexpected schedules: %q, %q", first.Schedule, second.Schedule)
	}
}

func TestListReturnsOnlyOpenTasks(t *testing.T) {
	store := tasks.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	svc := tasks.NewTaskService(store).WithClock(func() time.Time {
		return time.Date(2026, 2, 25, 9, 0, 0, 0, time.Local)
	})

	first, err := svc.CreateTomorrow("First")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := svc.CreateTomorrow("Second")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	status, err := svc.Complete(first.ID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if status != tasks.CompleteUpdated {
		t.Fatalf("complete status = %v, want %v", status, tasks.CompleteUpdated)
	}

	open, err := svc.ListOpen()
	if err != nil {
		t.Fatalf("list open: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("open task count = %d, want 1", len(open))
	}
	if open[0].ID != second.ID || open[0].Text != "Second" {
		t.Fatalf("unexpected open task: %+v", open[0])
	}
}

func TestCompleteUpdatesStatus(t *testing.T) {
	store := tasks.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	svc := tasks.NewTaskService(store).WithClock(func() time.Time {
		return time.Date(2026, 2, 25, 9, 0, 0, 0, time.Local)
	})

	created, err := svc.CreateTomorrow("Finish report")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	tests := []struct {
		name string
		id   int
		want tasks.CompleteStatus
	}{
		{name: "unknown", id: 999, want: tasks.CompleteUnknown},
		{name: "first complete", id: created.ID, want: tasks.CompleteUpdated},
		{name: "already done", id: created.ID, want: tasks.CompleteAlreadyDone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.Complete(tt.id)
			if err != nil {
				t.Fatalf("complete(%d): %v", tt.id, err)
			}
			if got != tt.want {
				t.Fatalf("complete(%d) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestPrepareDailyReminderRespectsStartAndLastReminded(t *testing.T) {
	store := tasks.NewStore(filepath.Join(t.TempDir(), "tasks.json"))
	svc := tasks.NewTaskService(store)
	yesterday := "2026-02-25"
	today := "2026-02-26"
	tomorrow := "2026-02-27"

	state := tasks.State{
		NextID: 6,
		Tasks: []tasks.Task{
			{ID: 1, Text: "a", StartDate: yesterday, Status: tasks.TaskStatusOpen, Schedule: "daily_6am"},
			{ID: 2, Text: "b", StartDate: today, Status: tasks.TaskStatusOpen, Schedule: "daily_6am", LastRemindedDate: &today},
			{ID: 3, Text: "c", StartDate: yesterday, Status: tasks.TaskStatusDone, Schedule: "daily_6am"},
			{ID: 4, Text: "d", StartDate: tomorrow, Status: tasks.TaskStatusOpen, Schedule: "daily_6am"},
			{ID: 5, Text: "e", StartDate: yesterday, Status: tasks.TaskStatusOpen, Schedule: "daily_6am", LastRemindedDate: &yesterday},
		},
	}
	if err := store.Save(state); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	selected, err := svc.PrepareDailyReminder(today)
	if err != nil {
		t.Fatalf("prepare reminder: %v", err)
	}

	gotIDs := []int{}
	for _, task := range selected {
		gotIDs = append(gotIDs, task.ID)
	}
	if !reflect.DeepEqual(gotIDs, []int{1, 5}) {
		t.Fatalf("selected ids = %v, want [1 5]", gotIDs)
	}

	selectedAgain, err := svc.PrepareDailyReminder(today)
	if err != nil {
		t.Fatalf("prepare second reminder: %v", err)
	}
	if len(selectedAgain) != 0 {
		t.Fatalf("selected again count = %d, want 0", len(selectedAgain))
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load after reminder: %v", err)
	}
	for _, task := range loaded.Tasks {
		if task.ID == 1 || task.ID == 5 {
			if task.LastRemindedDate == nil || *task.LastRemindedDate != today {
				t.Fatalf("task %d last_reminded_date = %v, want %q", task.ID, task.LastRemindedDate, today)
			}
		}
	}
}

func TestStoreLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	store := tasks.NewStore(path)
	d := "2026-02-26"

	want := tasks.State{
		NextID: 3,
		Tasks: []tasks.Task{
			{ID: 1, Text: "Buy eggs", CreatedAt: "2026-02-25T10:00:00-05:00", StartDate: "2026-02-26", Status: tasks.TaskStatusOpen, Schedule: "daily_6am", LastRemindedDate: nil},
			{ID: 2, Text: "Call landlord", CreatedAt: "2026-02-25T11:00:00-05:00", StartDate: "2026-02-26", Status: tasks.TaskStatusDone, Schedule: "daily_6am", LastRemindedDate: &d},
		},
	}

	if err := store.Save(want); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("roundtrip mismatch\n got: %#v\nwant: %#v", got, want)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("tasks.json perms = %o, want 600", info.Mode().Perm())
		}
	}
}
