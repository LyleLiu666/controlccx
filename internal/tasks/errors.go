package tasks

import (
	"fmt"
	"strings"
)

type WorkDirBusyError struct {
	WorkDir         string
	ExistingTaskID  string
	ExistingStatus  Status
	ExistingWorkDir string
}

func (e *WorkDirBusyError) Error() string {
	if e == nil {
		return "tasks: workdir is busy"
	}
	workdir := strings.TrimSpace(e.WorkDir)
	if workdir == "" {
		workdir = strings.TrimSpace(e.ExistingWorkDir)
	}
	if strings.TrimSpace(e.ExistingTaskID) == "" {
		if workdir == "" {
			return "tasks: workdir is busy"
		}
		return fmt.Sprintf("tasks: workdir is busy (workdir=%s)", workdir)
	}
	if workdir == "" {
		return fmt.Sprintf("tasks: workdir is busy (task_id=%s status=%s)", e.ExistingTaskID, e.ExistingStatus)
	}
	return fmt.Sprintf("tasks: workdir is busy (workdir=%s task_id=%s status=%s)", workdir, e.ExistingTaskID, e.ExistingStatus)
}
