package main

import (
	"log/slog"
	"os"

	"member/internal/server"
)

func main() {
	slog.Info("starting SigardaPanel Member Portal")
	if err := server.Run(); err != nil {
		slog.Error("member portal failed", "error", err)
		os.Exit(1)
	}
}
