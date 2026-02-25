package tasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	dateLayout       = "2006-01-02"
	scheduleDaily6AM = "daily_6am"
)

// TaskStatus tracks a task lifecycle.
type TaskStatus string

const (
	TaskStatusOpen TaskStatus = "open"
	TaskStatusDone TaskStatus = "done"
)

// Task is the persisted task schema.
type Task struct {
	ID               int        `json:"id"`
	Text             string     `json:"text"`
	CreatedAt        string     `json:"created_at"`
	StartDate        string     `json:"start_date"`
	Status           TaskStatus `json:"status"`
	Schedule         string     `json:"schedule"`
	LastRemindedDate *string    `json:"last_reminded_date"`
}

// State is the top-level tasks.json structure.
type State struct {
	NextID int    `json:"next_id"`
	Tasks  []Task `json:"tasks"`
}

// Store persists tasks in a single JSON file.
type Store struct {
	path string
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	return s.path
}

func (s *Store) Load() (State, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{NextID: 1, Tasks: []Task{}}, nil
		}
		return State{}, fmt.Errorf("read tasks file: %w", err)
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return State{NextID: 1, Tasks: []Task{}}, nil
	}

	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("parse tasks file: %w", err)
	}

	st = normalizeState(st)
	return st, nil
}

func (s *Store) Save(st State) (retErr error) {
	st = normalizeState(st)

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create tasks dir: %w", err)
	}

	tmp := s.path + ".tmp"
	defer func() {
		if retErr != nil {
			_ = os.Remove(tmp)
		}
	}()

	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open temp tasks file: %w", err)
	}

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(st); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp tasks file: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		return fmt.Errorf("fsync temp tasks file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp tasks file: %w", err)
	}

	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename temp tasks file: %w", err)
	}

	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod tasks file: %w", err)
	}

	return nil
}

func normalizeState(st State) State {
	if st.Tasks == nil {
		st.Tasks = []Task{}
	}
	if st.NextID < 1 {
		st.NextID = nextIDFromTasks(st.Tasks)
	}
	return st
}

func nextIDFromTasks(tasks []Task) int {
	next := 1
	for _, task := range tasks {
		if task.ID >= next {
			next = task.ID + 1
		}
	}
	return next
}
