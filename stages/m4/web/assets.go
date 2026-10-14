package web

import "embed"

//go:embed templates/*.html style.css
var Assets embed.FS
