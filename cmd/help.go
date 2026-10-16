package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const guidelinesFilename = ".todo-guidelines.txt"

func runHelp(stdout io.Writer, bold bool) error {
	dbFile, err := databaseFile()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(dbFile), guidelinesFilename)
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read local guidelines: %w", err)
	}

	var output strings.Builder
	output.WriteString(formatManual(bold))
	if strings.TrimSpace(string(content)) != "" {
		caption := "Local guidelines:"
		if bold {
			caption = "\x1b[1m" + caption + "\x1b[0m"
		}
		output.WriteString("\n" + caption + "\n\n")
		for _, line := range strings.SplitAfter(string(content), "\n") {
			if line != "" {
				output.WriteString("  " + line)
			}
		}
		if !strings.HasSuffix(string(content), "\n") {
			output.WriteByte('\n')
		}
	}
	_, err = io.WriteString(stdout, output.String())
	return err
}
