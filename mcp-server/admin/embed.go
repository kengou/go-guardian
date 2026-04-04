package admin

import "embed"

// UIAssets contains the built frontend SPA files.
// The ui/dist directory is populated by running the Vite build (npm run build)
// inside admin/ui/ before compiling the Go binary.
//
//go:embed ui/dist
var UIAssets embed.FS
