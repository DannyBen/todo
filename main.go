package main

import (
	"os"

	"github.com/dannyben/todo/cmd"
)

var Version = "0.0.0-dev"

func main() {
	if err := cmd.Execute(os.Args[1:], Version, os.Stdout); err != nil {
		cmd.PrintError(err, os.Stderr)
		os.Exit(1)
	}
}
