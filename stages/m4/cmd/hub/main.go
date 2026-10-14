package main

import (
	"github.com/win30221/hugofleet/stages/m4/hubapp"
	"log/slog"
	"os"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := hubapp.Run(hubapp.Options{}); err != nil {
		slog.Error("OIDC Hub stopped", "error", err)
		os.Exit(1)
	}
}
