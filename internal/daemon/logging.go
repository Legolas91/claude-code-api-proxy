package daemon

import (
	"io"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

const defaultLogFile = "cc-api-proxy.log"

// SetupLogging redirects os.Stdout and os.Stderr through a rotating log file.
// All fmt.Printf / fmt.Fprintf(os.Stderr, ...) output is captured and rotated.
// Config: 10MB max size, 3 backups, 7 days retention, gzip compression.
func SetupLogging() {
	logFile := filepath.Join(os.TempDir(), defaultLogFile)

	lj := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10, // MB
		MaxBackups: 3,
		MaxAge:     7,    // days
		Compress:   true, // gzip old logs
	}

	r, w, err := os.Pipe()
	if err != nil {
		return
	}

	os.Stdout = w
	os.Stderr = w

	go func() {
		_, _ = io.Copy(lj, r)
	}()
}
