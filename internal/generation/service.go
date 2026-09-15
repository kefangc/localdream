package generation

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Task struct {
	ID        string          `json:"id"`
	Prompt    string          `json:"prompt"`
	Status    string          `json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	Result    *ExecutorResult `json:"result,omitempty"` //omitempty表示为空值时不输出字段
	Error     string          `json:"error,omitempty"`
}

type Service struct {
	mu       sync.RWMutex
	tasks    map[string]Task
	nextID   int
	executor Executor
}

func NewService(executor Executor) *Service {
	return &Service{
		tasks:    make(map[string]Task),
		executor: executor,
	}
}

func (s *Service) Create(prompt string) (Task, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return Task{}, errors.New("empty prompt")
	}

	s.mu.Lock()

	s.nextID++

	task := Task{
		ID:        fmt.Sprintf("task_%d", s.nextID),
		Prompt:    prompt,
		Status:    "created",
		CreatedAt: time.Now(),
	}

	s.tasks[task.ID] = task

	s.mu.Unlock()

	go s.run(task.ID)

	return task, nil
}

func (s *Service) Get(id string) (Task, bool) {

	s.mu.RLock()
	defer s.mu.RUnlock()

	task, ok := s.tasks[id]
	return task, ok
}

func (s *Service) run(id string) {
	s.mu.Lock()

	task, ok := s.tasks[id]
	if !ok {
		s.mu.Unlock()
		return
	}

	task.Status = "running"
	s.tasks[id] = task

	s.mu.Unlock()

	result, err := s.executor.Execute(task.Prompt)

	s.mu.Lock()
	defer s.mu.Unlock()

	task = s.tasks[id]

	if err != nil {
		task.Status = "failed"
		task.Error = err.Error()
		s.tasks[id] = task
		return
	}

	task.Status = "completed"
	task.Result = &result
	s.tasks[id] = task
}
