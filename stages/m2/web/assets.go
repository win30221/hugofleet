// Package web embeds the M2 Go-template UI. No browser scripting is required.
package web

import "embed"

//go:embed templates/*.html style.css
var Assets embed.FS
