package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

var sessionLogNamePattern = regexp.MustCompile(`^\d{8}-\d{6}-\d+\.log$`)

func setupSessionLog() {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return
	}

	logDir := filepath.Join(cacheDir, "go_deep", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}

	pid := os.Getpid()
	rotateLogs(logDir, 7)

	logPath := filepath.Join(logDir, fmt.Sprintf("%s-%d.log", time.Now().Format("20060102-150405"), pid))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}

	log.SetOutput(file)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Llongfile)
}

func rotateLogs(logDir string, keep int) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	var logs []string
	for _, entry := range entries {
		if entry.IsDir() || !sessionLogNamePattern.MatchString(entry.Name()) {
			continue
		}
		logs = append(logs, filepath.Join(logDir, entry.Name()))
	}

	sort.Strings(logs)

	for len(logs) > keep {
		_ = os.Remove(logs[0])
		logs = logs[1:]
	}
}
