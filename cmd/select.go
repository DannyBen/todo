package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"unicode"

	"github.com/dannyben/todo/internal/store"
	"github.com/dannyben/todo/internal/task"
	"golang.org/x/sys/unix"
	"golang.org/x/term"
	"golang.org/x/text/width"
)

type selectionPrompt func([]task.Task, string, io.Reader, io.Writer) ([]int64, error)

func runSelection(database *store.Store, command string, filter *task.Selection, stdin io.Reader, stdout, stderr io.Writer, prompt editPrompt, selectPrompt selectionPrompt) error {
	items, err := database.List()
	if err != nil {
		return err
	}
	if filter != nil {
		ids, err := filter.Resolve(items)
		if err != nil {
			return err
		}
		matched := make(map[int64]bool, len(ids))
		for _, id := range ids {
			matched[id] = true
		}
		var matches []task.Task
		for _, item := range items {
			if matched[item.ID] {
				matches = append(matches, item)
			}
		}
		items = matches
	}
	if len(items) == 0 {
		_, err := fmt.Fprintln(stderr, "No tasks to select.")
		return err
	}
	ids, err := selectPrompt(items, command, stdin, stderr)
	if err != nil || len(ids) == 0 {
		return err
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	selector := strings.Join(parts, "/")
	if command == "del" {
		return runDelete(database, []string{selector}, stdout)
	}
	if len(ids) == 1 {
		return runInteractiveEdit(database, ids[0], stdin, stdout, stderr, prompt)
	}
	expression, err := prompt(selector, "", stdin, stderr)
	if errors.Is(err, errEditCanceled) {
		return nil
	}
	if err != nil || strings.TrimSpace(expression) == "" {
		return err
	}
	return runEdit(database, []string{selector, expression}, stdin, stdout, stderr, prompt, selectPrompt)
}

func selectTasks(items []task.Task, command string, stdin io.Reader, stderr io.Writer) (ids []int64, err error) {
	input, ok := stdin.(*os.File)
	output, outputTerminal := terminalWriter(stderr)
	if !ok || !term.IsTerminal(int(input.Fd())) || !outputTerminal {
		if command == "edit" {
			return nil, fmt.Errorf("interactive selection requires a terminal; provide a filter and edit expression after todo edit")
		}
		return nil, fmt.Errorf("interactive selection requires a terminal; provide a filter after todo %s", command)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, unix.SIGTERM, unix.SIGHUP, unix.SIGQUIT)
	defer stop()
	state, err := term.MakeRaw(int(input.Fd()))
	if err != nil {
		return nil, err
	}
	defer func() {
		_, screenErr := io.WriteString(stderr, "\x1b[?25h\x1b[?1049l")
		err = errors.Join(err, screenErr, term.Restore(int(input.Fd()), state))
	}()
	if _, err := io.WriteString(stderr, "\x1b[?1049h\x1b[?25l"); err != nil {
		return nil, err
	}
	selection := taskSelector{items: items, marked: make([]bool, len(items))}
	oldWidth, oldHeight := 0, 0
	dirty := true
	for {
		if ctx.Err() != nil {
			return nil, nil
		}
		w, h, err := term.GetSize(int(output.Fd()))
		if err != nil {
			return nil, err
		}
		if w < 2 || h < 2 {
			return nil, fmt.Errorf("interactive selection requires at least 2 terminal rows and columns")
		}
		if dirty || w != oldWidth || h != oldHeight {
			if _, err := io.WriteString(stderr, selection.render(command, w, h)); err != nil {
				return nil, err
			}
			oldWidth, oldHeight, dirty = w, h, false
		}
		key, err := selectionKey(input)
		if errors.Is(err, io.EOF) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if key == "" {
			continue
		}
		switch key {
		case "abort":
			return nil, nil
		case "enter":
			if ids := selection.ids(); len(ids) > 0 {
				return ids, nil
			}
		default:
			selection.move(key, max(1, h-1))
		}
		dirty = true
	}
}

// Polling lets a lone Escape cancel without consuming the first byte of an
// arrow key, and lets the screen follow resizes while no keys are pressed.
func selectionKey(input *os.File) (string, error) {
	read := func(timeout int) (byte, error) {
		fds := []unix.PollFd{{Fd: int32(input.Fd()), Events: unix.POLLIN}}
		n, err := unix.Poll(fds, timeout)
		if errors.Is(err, unix.EINTR) || n == 0 && err == nil {
			return 0, nil
		}
		if err != nil {
			return 0, err
		}
		var b [1]byte
		n, err = input.Read(b[:])
		if n == 0 && err == nil {
			err = io.EOF
		}
		return b[0], err
	}
	b, err := read(100)
	if err != nil {
		return "", err
	}
	switch b {
	case 3, 4, 'q':
		return "abort", nil
	case '\r', '\n':
		return "enter", nil
	case ' ':
		return "mark", nil
	case 27:
		b, err = read(75)
		if err != nil || b == 0 || b == 27 {
			return "abort", err
		}
		if b != '[' && b != 'O' {
			return "", nil
		}
		sequence := ""
		for range 16 {
			b, err = read(75)
			if err != nil || b == 0 {
				return "", err
			}
			sequence += string(b)
			if b >= 0x40 && b <= 0x7e {
				return map[string]string{"A": "up", "B": "down", "H": "home", "F": "end", "1~": "home", "4~": "end", "5~": "pageup", "6~": "pagedown", "7~": "home", "8~": "end"}[sequence], nil
			}
		}
	}
	return "", nil
}

type taskSelector struct {
	items  []task.Task
	marked []bool
	cursor int
	top    int
}

func (s *taskSelector) move(key string, page int) {
	switch key {
	case "up":
		s.cursor--
	case "down":
		s.cursor++
	case "home":
		s.cursor = 0
	case "end":
		s.cursor = len(s.items) - 1
	case "pageup":
		s.cursor -= page
	case "pagedown":
		s.cursor += page
	case "mark":
		s.marked[s.cursor] = !s.marked[s.cursor]
	}
	s.cursor = max(0, min(s.cursor, len(s.items)-1))
}

func (s *taskSelector) ids() []int64 {
	var ids []int64
	for i, marked := range s.marked {
		if marked {
			ids = append(ids, s.items[i].ID)
		}
	}
	return ids
}

func (s *taskSelector) render(command string, columns, rows int) string {
	color := os.Getenv("NO_COLOR") == ""
	page := max(1, rows-1)
	s.top = max(0, min(s.top, len(s.items)-page))
	if s.cursor < s.top {
		s.top = s.cursor
	}
	if s.cursor >= s.top+page {
		s.top = s.cursor - page + 1
	}
	var screen strings.Builder
	fmt.Fprintf(&screen, "\x1b[2J\x1b[%d;1H", max(1, rows-len(s.items)))
	for i := s.top; i < min(len(s.items), s.top+page); i++ {
		cursor, mark := " ", " "
		if i == s.cursor {
			cursor = ">"
		}
		if s.marked[i] {
			mark = "x"
		}
		box, text := "["+mark+"]", task.Format(s.items[i])
		if color {
			box = "\x1b[1m" + box + "\x1b[0m"
			if s.marked[i] {
				box = "\x1b[1;36m[x]\x1b[0m"
			}
			text = task.FormatColor(s.items[i])
		}
		screen.WriteString(selectorText(cursor+" "+box+" "+text, max(0, columns-1), color))
		screen.WriteString("\r\n")
	}
	verb := "delete"
	if command == "edit" {
		verb = "edit"
	}
	footer := selectorText("Space: mark for "+verb+"   Enter: confirm "+verb+"   Esc/q/Ctrl+C: abort", max(0, columns-1), false)
	if color {
		footer = highlightKeys(footer, "Space", "Enter", "Esc", "q", "Ctrl+C")
	}
	fmt.Fprintf(&screen, "\x1b[%d;1H%s", max(1, rows), footer)
	return screen.String()
}

// Keep every task on one row, including wide characters, and prevent stored
// control characters from moving the cursor. Only color/style escapes survive.
func selectorText(text string, columns int, color bool) string {
	var result strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if color && r == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			end := i + 2
			for end < len(runes) && (runes[end] >= '0' && runes[end] <= '9' || runes[end] == ';') {
				end++
			}
			if end < len(runes) && runes[end] == 'm' {
				result.WriteString(string(runes[i : end+1]))
				i = end
				continue
			}
		}
		if unicode.IsControl(r) {
			continue
		}
		cells := 1
		if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || r == '\u200d' {
			cells = 0
		} else if kind := width.LookupRune(r).Kind(); kind == width.EastAsianWide || kind == width.EastAsianFullwidth {
			cells = 2
		}
		if cells > columns {
			break
		}
		result.WriteRune(r)
		columns -= cells
	}
	if color {
		result.WriteString("\x1b[0m")
	}
	return result.String()
}
