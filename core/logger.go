package core

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Logger for debugging
type Logger struct {
	file *os.File
}

// NewLogger creates a new logger instance
func NewLogger() *Logger {
	logPath := filepath.Join(GetAppDataDir(), "dbswitcher.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return &Logger{}
	}
	return &Logger{file: file}
}

// Log writes a formatted message to the log file
func (l *Logger) Log(format string, args ...interface{}) {
	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		fmt.Fprintf(l.file, "[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
	}
}

// Close closes the log file
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}