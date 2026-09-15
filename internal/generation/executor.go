package generation

import (
	"fmt"
	"time"
)

type ExecutorResult struct {
	Simulated bool   `json:"simulated"`
	Message   string `json:"message"`
}

type Executor interface {
	Execute(prompt string) (ExecutorResult, error)
}

type FakeExecutor struct {
	Delay time.Duration
}

func (e *FakeExecutor) Execute(prompt string) (ExecutorResult, error) {
	time.Sleep(e.Delay)

	return ExecutorResult{
		Simulated: true,
		Message:   fmt.Sprintf("simulated success,receive prompt: %s", prompt),
	}, nil

}
