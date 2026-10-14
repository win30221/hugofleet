package main

import (
	"github.com/win30221/hugofleet/stages/m4/httpapi"
	"github.com/win30221/hugofleet/stages/m4/hubapp"
	"github.com/win30221/hugofleet/stages/m5/web"
	"log/slog"
	"os"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := hubapp.Run(hubapp.Options{Mode: "m5", Listen: "127.0.0.1:18095", Telemetry: true, API: httpapi.Options{Dashboard: true, Assets: web.Assets}}); err != nil {
		slog.Error("Dashboard Hub stopped", "error", err)
		os.Exit(1)
	}
}
