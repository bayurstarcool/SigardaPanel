package main

import (
	"os"

	"sigardapanel/internal/commands"
)

func main() {
	app := commands.New(os.Stdout, os.Stderr, os.Stdin)
	os.Exit(app.Run(os.Args[1:]))
}
