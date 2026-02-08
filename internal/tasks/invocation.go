package tasks

import "time"

type Invocation struct {
	TaskID          string    `json:"task_id"`
	Cmd             string    `json:"cmd"`
	Args            []string  `json:"args"`
	Dir             string    `json:"dir"`
	EnvInjectedKeys []string  `json:"env_injected_keys"`
	CreatedAt       time.Time `json:"created_at"`
}
