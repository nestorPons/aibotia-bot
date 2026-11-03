package whatsapp

import (
	"fmt"
	"log/slog"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// slogAdapter adapta *slog.Logger a la interfaz waLog.Logger.
type slogAdapter struct {
	log    *slog.Logger
	module string
}

func newWALogger(log *slog.Logger) waLog.Logger {
	return &slogAdapter{log: log}
}

func (a *slogAdapter) Sub(module string) waLog.Logger {
	name := module
	if a.module != "" {
		name = a.module + "." + module
	}
	return &slogAdapter{log: a.log, module: name}
}

func (a *slogAdapter) Debugf(msg string, args ...any) {
	a.log.Debug(a.format(msg, args...), "module", a.module)
}

func (a *slogAdapter) Infof(msg string, args ...any) {
	a.log.Info(a.format(msg, args...), "module", a.module)
}

func (a *slogAdapter) Warnf(msg string, args ...any) {
	a.log.Warn(a.format(msg, args...), "module", a.module)
}

func (a *slogAdapter) Errorf(msg string, args ...any) {
	a.log.Error(a.format(msg, args...), "module", a.module)
}

func (a *slogAdapter) format(msg string, args ...any) string {
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
