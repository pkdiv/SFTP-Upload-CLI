package uploader

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp    string `json:"timestamp"`
	RelPath      string `json:"relative_path"`
	LocalPath    string `json:"local_path"`
	RemotePath   string `json:"remote_path"`
	LocalSize    int64  `json:"local_size"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type SessionLog struct {
	mu   sync.Mutex
	file *os.File
}

func NewSessionLog(logDir string) (*SessionLog, error) {
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	name := fmt.Sprintf("uplift-%s.log", time.Now().Format("20060102-150405"))
	path := filepath.Join(logDir, name)

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return &SessionLog{file: f}, nil
}

func (l *SessionLog) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	return l.file.Close()
}

func (l *SessionLog) Record(entry LogEntry) error {
	if l == nil || l.file == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = l.file.Write(data)
	return err
}

func (l *SessionLog) Path() string {
	if l == nil || l.file == nil {
		return ""
	}
	return l.file.Name()
}

