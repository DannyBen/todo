package cmd

import (
	"bufio"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dannyben/todo/internal/store"
	"github.com/dannyben/todo/internal/task"
	"github.com/ergochat/readline"
	"golang.org/x/term"
)

const usage = `Todo - A lightweight project todo list

Usage:
  todo add TEXT...
  todo list FILTER...
  todo edit ID [TEXT...]
  todo del FILTER...
  todo help
`

//go:embed help/root.txt
var manual string

type usageError struct {
	message string
}

func (err usageError) Error() string { return err.message }

type editPrompt func(id int64, current string, stdin io.Reader, stderr io.Writer) (string, error)

var errEditCanceled = errors.New("edit canceled")

func Execute(args []string, version string, stdout io.Writer) error {
	return ExecuteWithIO(args, version, os.Stdin, stdout, os.Stderr)
}

func ExecuteWithIO(args []string, version string, stdin io.Reader, stdout, stderr io.Writer) error {
	return executeWithPrompt(args, version, stdin, stdout, stderr, promptEdit)
}

func executeWithPrompt(args []string, version string, stdin io.Reader, stdout, stderr io.Writer, prompt editPrompt) error {
	if len(args) == 0 {
		return usageError{message: strings.TrimSpace(usage)}
	}

	command := args[0]
	switch command {
	case "l", "ls":
		command = "list"
	case "a", "new":
		command = "add"
	case "d", "rm":
		command = "del"
	case "e":
		command = "edit"
	case "h":
		command = "help"
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
		return runAdd(database, args[1:], stdin, stdout)
	case "list":
		return runList(database, args[1:], stdout)
	case "edit":
		return runEdit(database, args[1:], stdin, stdout, stderr, prompt)
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

func runAdd(database *store.Store, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		if input, ok := stdin.(*os.File); ok && term.IsTerminal(int(input.Fd())) {
			return usageError{message: "usage: todo add TEXT..."}
		}
		return runAddInput(database, stdin, stdout)
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

func runAddInput(database *store.Store, stdin io.Reader, stdout io.Writer) error {
	var changes []task.Change
	var lines []int
	reader := bufio.NewReader(stdin)
	for line := 1; ; line++ {
		text, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return fmt.Errorf("line %d: read input: %w; no tasks added", line, err)
		}
		if strings.TrimSpace(text) != "" {
			change, parseErr := task.ParseChange([]string{text})
			if parseErr != nil {
				return fmt.Errorf("line %d: %w; no tasks added", line, parseErr)
			}
			if change.Description == nil || *change.Description == "" {
				return fmt.Errorf("line %d: task description cannot be empty; no tasks added", line)
			}
			changes = append(changes, change)
			lines = append(lines, line)
		}
		if err == io.EOF {
			break
		}
	}
	created, err := database.AddMany(changes)
	if len(created) > 0 {
		if outputErr := printTasks(stdout, created); outputErr != nil {
			return errors.Join(err, fmt.Errorf("tasks saved, but writing output failed: %w", outputErr))
		}
		return err
	}
	if err != nil {
		var itemErr *store.AddManyError
		if errors.As(err, &itemErr) {
			return fmt.Errorf("line %d: %w; no tasks added", lines[itemErr.Index], itemErr.Err)
		}
		return fmt.Errorf("%w; no tasks added", err)
	}
	return nil
}

func runList(database *store.Store, args []string, stdout io.Writer) error {
	matches, err := matchingTasks(database, args)
	if err != nil {
		return err
	}
	return printTasks(stdout, matches)
}

func matchingTasks(database *store.Store, args []string) ([]task.Task, error) {
	filters, err := task.ParseFilters(args)
	if err != nil {
		return nil, err
	}
	tasks, err := database.List()
	if err != nil {
		return nil, err
	}
	var matches []task.Task
	for _, item := range tasks {
		if filters.Match(item) {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func runEdit(database *store.Store, args []string, stdin io.Reader, stdout, stderr io.Writer, prompt editPrompt) error {
	if len(args) < 1 {
		return usageError{message: "usage: todo edit ID [TEXT...]"}
	}
	selection, err := task.ParseSelection(args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		if selection.Bulk() {
			return fmt.Errorf("multiple task IDs require an edit expression")
		}
		id, err := parseID(args[0])
		if err != nil {
			return err
		}
		return runInteractiveEdit(database, id, stdin, stdout, stderr, prompt)
	}
	change, err := task.ParseChange(args[1:])
	if err != nil {
		return err
	}
	if selection.Bulk() && change.Description != nil {
		return fmt.Errorf("bulk edits cannot replace task descriptions; use only tag and connection operations")
	}
	items, err := database.List()
	if err != nil {
		return err
	}
	ids, err := selection.Resolve(items)
	if err != nil {
		return err
	}
	return applyChanges(database, ids, change, stdout)
}

func applyEdit(database *store.Store, id int64, args []string, stdout io.Writer) error {
	change, err := task.ParseChange(args)
	if err != nil {
		return err
	}
	return applyChanges(database, []int64{id}, change, stdout)
}

func applyChanges(database *store.Store, ids []int64, change task.Change, stdout io.Writer) error {
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

func runInteractiveEdit(database *store.Store, id int64, stdin io.Reader, stdout, stderr io.Writer, prompt editPrompt) error {
	item, err := database.Get(id)
	if err != nil {
		return err
	}
	edited, err := prompt(id, task.Expression(item), stdin, stderr)
	if errors.Is(err, errEditCanceled) {
		return nil
	}
	if err != nil {
		return err
	}
	return applyEdit(database, id, strings.Fields(edited), stdout)
}

func promptEdit(id int64, current string, stdin io.Reader, stderr io.Writer) (edited string, err error) {
	input, inputIsTerminal := stdin.(*os.File)
	_, outputIsTerminal := terminalWriter(stderr)
	if !inputIsTerminal || !term.IsTerminal(int(input.Fd())) || !outputIsTerminal {
		return "", fmt.Errorf("interactive edit requires a terminal; provide edit text after the task ID")
	}

	hint, prompt := editPromptText(id, os.Getenv("NO_COLOR") == "")
	if _, err := fmt.Fprintf(stderr, "%s\n\n", hint); err != nil {
		return "", err
	}

	line, err := readline.NewEx(&readline.Config{
		Prompt:                 prompt,
		Stdin:                  stdin,
		Stdout:                 stderr,
		Stderr:                 stderr,
		HistoryLimit:           -1,
		DisableAutoSaveHistory: true,
		InterruptPrompt:        "\n",
		EOFPrompt:              "\n",
		FuncIsTerminal:         func() bool { return true },
	})
	if err != nil {
		return "", fmt.Errorf("start inline editor: %w", err)
	}
	defer func() {
		if closeErr := line.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close inline editor: %w", closeErr)
		}
	}()

	edited, err = line.ReadLineWithDefault(current)
	if errors.Is(err, readline.ErrInterrupt) || errors.Is(err, io.EOF) {
		return "", errEditCanceled
	}
	if err != nil {
		return "", fmt.Errorf("read inline edit: %w", err)
	}
	return edited, nil
}

func editPromptText(id int64, color bool) (hint, prompt string) {
	hint = "Enter saves · Ctrl+C cancels"
	prompt = strconv.FormatInt(id, 10) + " "
	if color {
		hint = "\x1b[1;36mEnter\x1b[0m saves · \x1b[1;36mCtrl+C\x1b[0m cancels"
		prompt = "\x1b[1;33m" + strconv.FormatInt(id, 10) + "\x1b[0m "
	}
	return hint, prompt
}

func runDelete(database *store.Store, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return usageError{message: "usage: todo del FILTER..."}
	}
	if selection, err := task.ParseSelection(args[0]); len(args) == 1 && err == nil {
		items, err := database.List()
		if err != nil {
			return err
		}
		ids, err := selection.Resolve(items)
		if err != nil {
			return err
		}
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

	matches, err := matchingTasks(database, args)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		filters, err := task.ParseFilters(args)
		if err != nil {
			return err
		}
		if filters.HasRanges() {
			return fmt.Errorf("no tasks match selection")
		}
	}
	ids := make([]int64, len(matches))
	for index, item := range matches {
		ids[index] = item.ID
	}
	deleted, err := database.DeleteMany(ids)
	if err != nil {
		return err
	}
	return printTasks(stdout, deleted)
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
	if configured := os.Getenv("TODO_FILE"); configured != "" {
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
