# Agent Notes

## Project Shape

- `main.go` is the small binary entrypoint and injects `Version`.
- `cmd/` owns CLI parsing, command behavior, output, and CLI tests.
- `internal/task/` owns inner-syntax parsing, filtering, and formatting.
- `internal/store/` owns SQLite persistence and transactions.
- `op.conf` is the user-facing command catalog.

## Useful Commands

```bash
go test ./...
op check
go run . --help
```

When completing a feature, run `op check`, run `op build`, and then mark its
todo with `+done`. Do not delete the task.

Before selecting a task, run `todo list +now`. If it returns any tasks, choose
from those before considering the rest of the list.

## Editing Rules

- Keep the CLI flag-free; leading punctuation belongs to the inner syntax.
- Agents must not delete tasks unless the user explicitly asks them to.
- Keep stdout stable and scriptable; errors go to stderr.
- Do not edit `.todo.sqlite` directly; use the CLI.
- Do not run parallel commands that walk files under `/vagrant`.
