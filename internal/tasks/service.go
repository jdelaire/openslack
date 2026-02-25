package tasks

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrEmptyTaskText = errors.New("task text is empty")

// CompleteStatus describes the result of marking a task as done.
type CompleteStatus int

const (
	CompleteUpdated CompleteStatus = iota
	CompleteUnknown
	CompleteAlreadyDone
)

// TaskService provides task CRUD and reminder selection logic.
type TaskService struct {
	store *Store
	now   func() time.Time
	mu    sync.Mutex
}

func NewTaskService(store *Store) *TaskService {
	return &TaskService{
		store: store,
		now:   time.Now,
	}
}

func (s *TaskService) WithClock(now func() time.Time) *TaskService {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *TaskService) CreateTomorrow(text string) (Task, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Task{}, ErrEmptyTaskText
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.store.Load()
	if err != nil {
		return Task{}, err
	}

	now := s.now().In(time.Local)
	tomorrowDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

	id := st.NextID
	if id < 1 {
		id = nextIDFromTasks(st.Tasks)
	}

	task := Task{
		ID:               id,
		Text:             text,
		CreatedAt:        now.Format(time.RFC3339),
		StartDate:        tomorrowDate.Format(dateLayout),
		Status:           TaskStatusOpen,
		Schedule:         scheduleDaily6AM,
		LastRemindedDate: nil,
	}

	st.NextID = id + 1
	st.Tasks = append(st.Tasks, task)
	if err := s.store.Save(st); err != nil {
		return Task{}, err
	}

	return task, nil
}

func (s *TaskService) ListOpen() ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	open := make([]Task, 0, len(st.Tasks))
	for _, task := range st.Tasks {
		if task.Status == TaskStatusOpen {
			open = append(open, task)
		}
	}
	sort.Slice(open, func(i, j int) bool {
		return open[i].ID < open[j].ID
	})
	return open, nil
}

func (s *TaskService) Complete(id int) (CompleteStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.store.Load()
	if err != nil {
		return CompleteUnknown, err
	}

	idx := -1
	for i := range st.Tasks {
		if st.Tasks[i].ID == id {
			idx = i
			break
		}
	}

	if idx == -1 {
		return CompleteUnknown, nil
	}
	if st.Tasks[idx].Status == TaskStatusDone {
		return CompleteAlreadyDone, nil
	}

	st.Tasks[idx].Status = TaskStatusDone
	if err := s.store.Save(st); err != nil {
		return CompleteUnknown, err
	}

	return CompleteUpdated, nil
}

// PrepareDailyReminder returns tasks that should be reminded today.
// It sets and persists last_reminded_date before returning the tasks.
func (s *TaskService) PrepareDailyReminder(today string) ([]Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, err := s.store.Load()
	if err != nil {
		return nil, err
	}

	selected := make([]Task, 0)
	selectedIdx := make([]int, 0)
	for i := range st.Tasks {
		task := st.Tasks[i]
		if task.Status != TaskStatusOpen {
			continue
		}
		if task.StartDate > today {
			continue
		}
		if task.LastRemindedDate != nil && *task.LastRemindedDate == today {
			continue
		}

		selected = append(selected, task)
		selectedIdx = append(selectedIdx, i)
	}

	if len(selected) == 0 {
		return nil, nil
	}

	for _, idx := range selectedIdx {
		mark := today
		st.Tasks[idx].LastRemindedDate = &mark
	}

	if err := s.store.Save(st); err != nil {
		return nil, fmt.Errorf("persist reminder marks: %w", err)
	}

	sort.Slice(selected, func(i, j int) bool {
		return selected[i].ID < selected[j].ID
	})
	return selected, nil
}
