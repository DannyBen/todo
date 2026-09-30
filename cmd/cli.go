package cmd

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dannyben/todo/internal/store"
	"github.com/dannyben/todo/internal/task"
	"golang.org/x/term"
)

const usage = `Todo - A lightweight project todo list

Usage:
  todo add TEXT...
  todo list FILTER...
  todo edit ID[/ID...] [TEXT...]
  todo del ID[/ID...]|+TAG
  todo help
`

//go:embed help/root.txt
var manual string

type usageError struct {
	message string
}

func (err usageError) Error() string { return err.message }

func Execute(args []string, version string, stdout io.Writer) error {
	return ExecuteWithIO(args, version, os.Stdin, stdout, os.Stderr)
}

func ExecuteWithIO(args []string, version string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return usageError{message: strings.TrimSpace(usage)}
	}

	command := args[0]
	switch command {
	case "ls":
		command = "list"
	case "new":
		command = "add"
	case "rm":
		command = "del"
	}

	switch command {
	case "help", "--help", "-h":
		_, terminal := terminalWriter(stdout)
		_, err := fmt.Fprint(stdout, formatManual(terminal && os.Getenv("NO_COLOR") == ""))
		return err
	case "version", "--version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "add", "list", "edit", "del":
		// Continue below once the command name is known to be valid.
	default:
		return usageError{message: fmt.Sprintf("unknown command %q\n\n%s", args[0], strings.TrimSpace(usage))}
	}

	dbFile, err := databaseFile()
	if err != nil {
		return err
	}
	database, err := store.Open(dbFile)
	if err != nil {
		return err
	}
	defer database.Close()

	switch command {
	case "add":
		return runAdd(database, args[1:], stdout)
	case "list":
		return runList(database, args[1:], stdout)
	case "edit":
		return runEdit(database, args[1:], stdin, stdout, stderr)
	case "del":
		return runDelete(database, args[1:], stdout)
	}
	return nil
}

func formatManual(bold bool) string {
	lines := strings.Split(manual, "\n")
	for index, line := range lines {
		if len(line) >= 4 && strings.HasPrefix(line, "**") && strings.HasSuffix(line, "**") {
			caption := strings.TrimSuffix(strings.TrimPrefix(line, "**"), "**")
			if bold {
				caption = "\x1b[1m" + caption + "\x1b[0m"
			}
			lines[index] = caption
		}
	}
	return strings.Join(lines, "\n")
}

func PrintError(err error, stderr io.Writer) {
	if err == nil {
		return
	}
	var usage usageError
	if errors.As(err, &usage) {
		fmt.Fprintln(stderr, usage.Error())
		return
	}
	fmt.Fprintf(stderr, "error: %v\n", err)
}

func runAdd(database *store.Store, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return usageError{message: "usage: todo add TEXT..."}
	}
	change, err := task.ParseChange(args)
	if err != nil {
		return err
	}
	if change.Description == nil || *change.Description == "" {
		return fmt.Errorf("task description cannot be empty")
	}
	created, err := database.Add(*change.Description, change.AddTags, change.AddReferences)
	if err != nil {
		return err
	}
	return printTasks(stdout, []task.Task{created})
}

func runList(database *store.Store, args []string, stdout io.Writer) error {
	filters, err := task.ParseFilters(args)
	if err != nil {
		return err
	}
	tasks, err := database.List()
	if err != nil {
		return err
	}
	var matches []task.Task
	for _, item := range tasks {
		if filters.Match(item) {
			matches = append(matches, item)
		}
	}
	return printTasks(stdout, matches)
}

func runEdit(database *store.Store, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) < 1 {
		return usageError{message: "usage: todo edit ID[/ID...] [TEXT...]"}
	}
	ids, err := parseIDs(args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		if len(ids) != 1 {
			return fmt.Errorf("multiple task IDs require an edit expression")
		}
		return runEditor(database, ids[0], stdin, stdout, stderr)
	}
	return applyEdits(database, ids, args[1:], stdout)
}

func applyEdit(database *store.Store, id int64, args []string, stdout io.Writer) error {
	return applyEdits(database, []int64{id}, args, stdout)
}

func applyEdits(database *store.Store, ids []int64, args []string, stdout io.Writer) error {
	change, err := task.ParseChange(args)
	if err != nil {
		return err
	}
	if len(ids) == 1 {
		updated, err := database.Edit(ids[0], change)
		if err != nil {
			return err
		}
		return printTasks(stdout, []task.Task{updated})
	}
	updated, err := database.EditMany(ids, change)
	if err != nil {
		return err
	}
	return printTasks(stdout, updated)
}

func runEditor(database *store.Store, id int64, stdin io.Reader, stdout, stderr io.Writer) error {
	item, err := database.Get(id)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "todo-edit-*.txt")
	if err != nil {
		return fmt.Errorf("create editor file: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := fmt.Fprintln(file, task.Expression(item)); err != nil {
		file.Close()
		return fmt.Errorf("write editor file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close editor file: %w", err)
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}
	command := exec.Command("sh", "-c", editor+" \"$1\"", "todo-edit", path)
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run editor: %w", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read editor file: %w", err)
	}
	return applyEdit(database, id, strings.Fields(string(content)), stdout)
}

func runDelete(database *store.Store, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return usageError{message: "usage: todo del ID[/ID...]|+TAG"}
	}
	ids, err := parseIDs(args[0])
	if err == nil {
		if len(ids) == 1 {
			deleted, err := database.Delete(ids[0])
			if err != nil {
				return err
			}
			return printTasks(stdout, []task.Task{deleted})
		}
		deleted, err := database.DeleteMany(ids)
		if err != nil {
			return err
		}
		return printTasks(stdout, deleted)
	}

	filters, filterErr := task.ParseFilters(args)
	if filterErr == nil && len(filters.IncludeTags) == 1 && len(filters.Terms) == 0 &&
		len(filters.ExcludeTags) == 0 && len(filters.IncludeReferences) == 0 &&
		len(filters.ExcludeReferences) == 0 && len(filters.IDs) == 0 {
		deleted, err := database.DeleteByTag(filters.IncludeTags[0])
		if err != nil {
			return err
		}
		return printTasks(stdout, deleted)
	}
	return err
}

func parseID(value string) (int64, error) {
	ids, err := parseIDs(value)
	if err != nil || len(ids) != 1 {
		return 0, fmt.Errorf("invalid task ID %q", value)
	}
	return ids[0], nil
}

func parseIDs(value string) ([]int64, error) {
	ids, ok := task.ParseIDs(value)
	if !ok {
		return nil, fmt.Errorf("invalid task ID %q", value)
	}
	return ids, nil
}

func printTasks(stdout io.Writer, tasks []task.Task) error {
	indent := 0
	for _, item := range tasks {
		indent = max(indent, len(strconv.FormatInt(item.ID, 10))+1)
	}
	for _, item := range tasks {
		if _, err := fmt.Fprintln(stdout, formatTask(item, stdout, indent)); err != nil {
			return err
		}
	}
	return nil
}

func formatTask(item task.Task, stdout io.Writer, indent int) string {
	file, terminal := terminalWriter(stdout)
	if !terminal {
		return task.Format(item)
	}
	color := os.Getenv("NO_COLOR") == ""
	if width, _, err := term.GetSize(int(file.Fd())); err == nil && width > 0 {
		return task.FormatWrapped(item, width, indent, color)
	}
	if color {
		return task.FormatColor(item)
	}
	return task.Format(item)
}

func terminalWriter(writer io.Writer) (*os.File, bool) {
	file, ok := writer.(*os.File)
	return file, ok && term.IsTerminal(int(file.Fd()))
}

func databaseFile() (string, error) {
	if configured := os.Getenv("TODO_DB_FILE"); configured != "" {
		return configured, nil
	}

	root, err := projectRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".todo.sqlite"), nil
}

func projectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current directory: %w", err)
	}
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect Git root: %w", err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return start, nil
		}
		dir = parent
	}
}
