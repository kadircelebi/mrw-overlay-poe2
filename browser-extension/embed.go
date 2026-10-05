// Package browserextension contains the browser bridge shipped with the app.
package browserextension

import "embed"

// Files contains only browser assets; Go sources are never copied to the extension.
//
//go:embed *.js *.json icons
var Files embed.FS
