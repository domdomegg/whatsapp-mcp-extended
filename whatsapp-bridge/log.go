package main

import (
	"fmt"
	"os"
	"time"

	waLog "go.mau.fi/whatsmeow/util/log"
)

// stderrLogger is a waLog.Logger that writes to stderr. stdout is the MCP
// channel, so nothing else may ever write there.
type stderrLogger struct {
	module string
	debug  bool
}

func newLogger(module string, debug bool) waLog.Logger {
	return &stderrLogger{module: module, debug: debug}
}

func (l *stderrLogger) logf(level, msg string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s [%s %s] %s\n", time.Now().Format("15:04:05.000"), l.module, level, fmt.Sprintf(msg, args...))
}

func (l *stderrLogger) Warnf(msg string, args ...any)  { l.logf("WARN", msg, args...) }
func (l *stderrLogger) Errorf(msg string, args ...any) { l.logf("ERROR", msg, args...) }
func (l *stderrLogger) Infof(msg string, args ...any)  { l.logf("INFO", msg, args...) }
func (l *stderrLogger) Debugf(msg string, args ...any) {
	if l.debug {
		l.logf("DEBUG", msg, args...)
	}
}
func (l *stderrLogger) Sub(module string) waLog.Logger {
	return &stderrLogger{module: l.module + "/" + module, debug: l.debug}
}
