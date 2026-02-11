package taskops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"controlccx/internal/runsafe"
	"controlccx/internal/tasks"
	"controlccx/internal/worktree"

	"github.com/google/uuid"
)

func (s *Service) CreateTask(ctx context.Context, in tasks.CreateTaskInput) (tasks.Task, error) {
	if s == nil || s.Tasks == nil {
		return tasks.Task{}, newMutationError(503, MutationErrorInternal, "tasks store not configured", "", nil, nil)
	}
	if strings.TrimSpace(string(in.WorkerType)) == "" {
		return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "tasks: worker_type is required", "", nil, nil)
	}

	if s.Tools != nil {
		if _, ok := s.Tools.Resolve(string(in.WorkerType)); !ok {
			msg := "unknown tool id: " + string(in.WorkerType)
			return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, msg, "", nil, nil)
		}
	}

	driver := in.WorkerType
	if s.Tools != nil {
		if profile, ok := s.Tools.Resolve(string(in.WorkerType)); ok && strings.TrimSpace(string(profile.Driver)) != "" {
			driver = tasks.WorkerType(strings.TrimSpace(string(profile.Driver)))
		}
	}
	envelope := runsafe.SafetyEnvelope(strings.TrimSpace(in.SafetyEnvelope))
	in, ap := runsafe.ApplyAutopilot(ctx, in, runsafe.ApplyOptions{
		Driver:   driver,
		Envelope: envelope,
		Classify: runsafe.ClassifyOptions{},
	})

	var worktreePrepLogs []string
	strategy := strings.ToLower(strings.TrimSpace(in.WorkDirStrategy))
	if strategy == "worktree" {
		if in.Mode != tasks.ModeNew {
			return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "workdir_strategy=worktree is only supported for mode=new", "", nil, nil)
		}

		// Preserve idempotency semantics: do not create a new worktree for replays.
		if k := strings.TrimSpace(in.IdempotencyKey); k != "" {
			existing, ok, err := s.Tasks.GetTaskByIdempotencyKey(ctx, k)
			if err != nil {
				return tasks.Task{}, err
			}
			if ok {
				return existing, nil
			}
		}

		cid := strings.TrimSpace(in.ConversationID)
		if cid == "" {
			cid = uuid.NewString()
		} else {
			parsed, err := uuid.Parse(cid)
			if err != nil {
				return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "conversation_id must be a UUID for workdir_strategy=worktree", "", nil, err)
			}
			cid = parsed.String()
		}
		in.ConversationID = cid

		base := strings.TrimSpace(in.WorkDir)
		if base == "" {
			base = "."
		}
		untracked := strings.ToLower(strings.TrimSpace(in.WorktreeUntracked))
		var untrackedMode worktree.UntrackedMode
		switch untracked {
		case "":
			untrackedMode = worktree.UntrackedModeDefault
		case "skip":
			untrackedMode = worktree.UntrackedModeSkip
		case "force":
			untrackedMode = worktree.UntrackedModeForce
		default:
			return tasks.Task{}, newMutationError(400, MutationErrorInvalidArgument, "worktree_untracked must be one of: skip, force", "", nil, nil)
		}

		wt, err := worktree.Create(ctx, worktree.CreateOptions{
			BaseWorkDir:    base,
			ConversationID: cid,
			Untracked:      untrackedMode,
			Logf: func(format string, args ...any) {
				worktreePrepLogs = append(worktreePrepLogs, fmt.Sprintf(format, args...))
			},
		})
		if err != nil {
			var tooLarge *worktree.UntrackedTooLargeError
			if errors.As(err, &tooLarge) {
				return tasks.Task{}, newMutationError(422, MutationErrorInvalidArgument, err.Error(), "", map[string]any{
					"reason":          "worktree_untracked_too_large",
					"conversation_id": cid,
					"files":           tooLarge.Files,
					"bytes":           tooLarge.Bytes,
					"max_files":       tooLarge.MaxFiles,
					"max_bytes":       tooLarge.MaxBytes,
					"largest":         tooLarge.Largest,
				}, err)
			}
			return tasks.Task{}, err
		}
		in.WorkDirStrategy = "worktree"
		in.BaseWorkDir = strings.TrimSpace(wt.RepoRoot)
		in.WorktreeDir = strings.TrimSpace(wt.Dir)
		in.WorktreeBranch = strings.TrimSpace(wt.Branch)
		in.WorkDir = wt.Dir
	}

	task, err := s.Tasks.CreateTask(ctx, in)
	if err != nil {
		return tasks.Task{}, err
	}

	for _, msg := range worktreePrepLogs {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			continue
		}
		_, _ = s.Tasks.AppendLog(ctx, task.ID, tasks.LogSystem, msg)
	}
	if ap.Applied {
		if audit := runsafe.FormatAuditLog(driver, ap.Decision, in, true); strings.TrimSpace(audit) != "" {
			_, _ = s.Tasks.AppendLog(ctx, task.ID, tasks.LogSystem, audit)
		}
	}

	// Keep behavior aligned with existing API create path: start immediately.
	return s.startTask(ctx, task)
}
