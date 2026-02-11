package tools

import "errors"

func errorsNewTaskIDRequired() error {
	return errors.New("task_id is required")
}
