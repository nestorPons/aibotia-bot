// Package version expone metadatos de build inyectables con ldflags.
package version

// Valores por defecto; se sobreescriben en build:
//
//	go build -ldflags "-X .../internal/version.Version=1.0.0 ..."
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)
