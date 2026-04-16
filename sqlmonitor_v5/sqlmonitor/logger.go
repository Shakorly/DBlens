package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type LogLevel int

const (
	levelDebug LogLevel = iota
	levelInfo
	levelWarn
	levelError
)

// Logger writes structured timestamped entries to stdout + a rotating log file.
// Rotation triggers on: new calendar day OR file exceeding maxSizeMB.
// Old files are optionally gzip-compressed and pruned after maxFiles days.
type Logger struct {
	mu          sync.Mutex
	level       LogLevel
	baseFile    string   // e.g. "sql_monitor.log"
	maxSizeMB   int64
	maxFiles    int
	compress    bool
	currentFile *os.File
	currentDate string
	currentSize int64
	stdLogger   *log.Logger
}

func NewLogger(logFile, logLevel string) (*Logger, error) {
	l := &Logger{
		baseFile:  logFile,
		maxSizeMB: 50,   // 50 MB per file
		maxFiles:  30,   // keep 30 rotated files
		compress:  true, // gzip old files
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
	if err := l.openFile(); err != nil {
		return nil, err
	}
	return l, nil
}

// openFile opens (or creates) today's log file.
func (l *Logger) openFile() error {
	today := time.Now().Format("2006-01-02")
	path := l.dailyPath(today)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("cannot open log file %q: %w", path, err)
	}
	info, _ := f.Stat()
	if l.currentFile != nil {
		_ = l.currentFile.Close()
	}
	l.currentFile = f
	l.currentDate = today
	if info != nil {
		l.currentSize = info.Size()
	}
	writer := io.MultiWriter(os.Stdout, f)
	l.stdLogger = log.New(writer, "", 0)
	return nil
}

// dailyPath returns the log file path for a given date.
func (l *Logger) dailyPath(date string) string {
	ext := filepath.Ext(l.baseFile)
	base := strings.TrimSuffix(l.baseFile, ext)
	if date == time.Now().Format("2006-01-02") {
		return l.baseFile // today = canonical file
	}
	return fmt.Sprintf("%s_%s%s", base, date, ext)
}

// rotate checks if a new file should be opened and compresses the old one.
func (l *Logger) rotate() {
	today := time.Now().Format("2006-01-02")
	needRotate := today != l.currentDate || l.currentSize >= l.maxSizeMB*1024*1024

	if !needRotate {
		return
	}

	oldPath := l.currentFile.Name()
	_ = l.currentFile.Close()
	l.currentFile = nil

	// Rename current file to dated archive
	if l.currentDate != "" && l.currentDate != today {
		dated := l.dailyPath(l.currentDate)
		if dated != oldPath {
			_ = os.Rename(oldPath, dated)
			oldPath = dated
		}
	} else if l.currentSize >= l.maxSizeMB*1024*1024 {
		ts := time.Now().Format("2006-01-02_150405")
		ext := filepath.Ext(l.baseFile)
		base := strings.TrimSuffix(l.baseFile, ext)
		sized := fmt.Sprintf("%s_%s%s", base, ts, ext)
		_ = os.Rename(oldPath, sized)
		oldPath = sized
	}

	// Compress old file
	if l.compress && strings.HasSuffix(oldPath, ".log") {
		go compressFile(oldPath)
	}

	// Prune old files
	go l.pruneOldFiles()

	// Open fresh file
	_ = l.openFile()
}

func compressFile(path string) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(path + ".gz")
	if err != nil {
		return
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	_, _ = io.Copy(gz, in)
	_ = gz.Close()
	_ = os.Remove(path)
}

func (l *Logger) pruneOldFiles() {
	ext := filepath.Ext(l.baseFile)
	base := strings.TrimSuffix(filepath.Base(l.baseFile), ext)
	dir := filepath.Dir(l.baseFile)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -l.maxFiles)
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base+"_") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}

func (l *Logger) write(level LogLevel, levelTag, server, msg string) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotate()
	ts := time.Now().Format("2006-01-02 15:04:05")
	srvTag := fmt.Sprintf("[%-14s] ", server)
	if server == "" {
		srvTag = fmt.Sprintf("[%-14s] ", "SYSTEM")
	}
	line := fmt.Sprintf("%s [%s] %s%s", ts, levelTag, srvTag, msg)
	l.stdLogger.Println(line)
	l.currentSize += int64(len(line)) + 1
}

func (l *Logger) Debug(server, msg string) { l.write(levelDebug, "DEBUG", server, msg) }
func (l *Logger) Info(server, msg string)  { l.write(levelInfo, "INFO ", server, msg) }
func (l *Logger) Warn(server, msg string)  { l.write(levelWarn, "WARN ", server, msg) }
func (l *Logger) Error(server, msg string) { l.write(levelError, "ERROR", server, msg) }

func (l *Logger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.currentFile != nil {
		_ = l.currentFile.Close()
	}
}
