package web

import "embed"

// Assets override the layout while keeping secured M4 form handlers.
//
//go:embed templates/*.html style.css
var Assets embed.FS
