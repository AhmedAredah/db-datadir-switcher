package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogLevel represents the logging level
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
)

// String returns the string representation of the log level
func (l LogLevel) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	default:
		return "INFO"
	}
}

// ParseLogLevel converts a string to LogLevel
func ParseLogLevel(level string) LogLevel {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	default:
		return INFO
	}
}

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

// Log writes a formatted message to the log file (defaults to INFO level)
func (l *Logger) Log(format string, args ...interface{}) {
	l.LogLevel(INFO, format, args...)
}

// LogLevel writes a formatted message with specific log level
func (l *Logger) LogLevel(level LogLevel, format string, args ...interface{}) {
	// Check if logging is enabled for this level
	if !l.shouldLog(level) {
		return
	}
	
	if l.file != nil {
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		message := fmt.Sprintf(format, args...)
		
		// Add extra context for verbose logging
		if AppConfig.VerboseLogging {
			// Add more detailed formatting for verbose mode
			fmt.Fprintf(l.file, "[%s] [%s] %s\n", timestamp, level.String(), message)
		} else {
			fmt.Fprintf(l.file, "[%s] %s\n", timestamp, message)
		}
		
		// Force flush for debug mode to ensure immediate writing
		if AppConfig.DebugMode {
			l.file.Sync()
		}
	}
}

// shouldLog determines if a message should be logged based on configured level
func (l *Logger) shouldLog(level LogLevel) bool {
	configLevel := ParseLogLevel(AppConfig.LogLevel)
	return level >= configLevel
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.LogLevel(DEBUG, format, args...)
}

// Info logs an info message  
func (l *Logger) Info(format string, args ...interface{}) {
	l.LogLevel(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.LogLevel(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.LogLevel(ERROR, format, args...)
}

// Close closes the log file
func (l *Logger) Close() {
	if l.file != nil {
		l.file.Close()
	}
}