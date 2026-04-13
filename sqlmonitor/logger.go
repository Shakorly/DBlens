package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"
)

// LogLevel represents severity tiers for log filtering.
type LogLevel int

const (
	levelDebug LogLevel = iota
	levelInfo
	levelWarn
	levelError
)

// Logger writes structured timestamped entries to both stdout and a log file.
type Logger struct {
	level  LogLevel
	logger *log.Logger
	file   *os.File
}

// NewLogger creates a Logger that writes to logFile and stdout simultaneously.
func NewLogger(logFile, logLevel string) (*Logger, error) {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file %q: %w", logFile, err)
	}

	writer := io.MultiWriter(os.Stdout, f)
	l := &Logger{
		logger: log.New(writer, "", 0),
		file:   f,
	}

	switch strings.ToUpper(logLevel) {
	case "DEBUG":
		l.level = levelDebug
	case "WARN":
		l.level = levelWarn
	case "ERROR":
		l.level = levelError
	default:
		l.level = levelInfo
	}

	return l, nil
}

func (l *Logger) write(level LogLevel, levelTag, server, msg string) {
	if level < l.level {
		return
	}
	ts := time.Now().Format("2006-01-02 15:04:05")
	srvTag := ""
	if server != "" {
		srvTag = fmt.Sprintf("[%-14s] ", server)
	} else {
		srvTag = fmt.Sprintf("[%-14s] ", "SYSTEM")
	}
	l.logger.Printf("%s [%s] %s%s", ts, levelTag, srvTag, msg)
}

func (l *Logger) Debug(server, msg string) { l.write(levelDebug, "DEBUG", server, msg) }
func (l *Logger) Info(server, msg string)  { l.write(levelInfo, "INFO ", server, msg) }
func (l *Logger) Warn(server, msg string)  { l.write(levelWarn, "WARN ", server, msg) }
func (l *Logger) Error(server, msg string) { l.write(levelError, "ERROR", server, msg) }

// Close flushes and closes the underlying log file.
func (l *Logger) Close() {
	if l.file != nil {
		_ = l.file.Close()
	}
}
