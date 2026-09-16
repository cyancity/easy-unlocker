package broker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEvent struct {
	Time      string `json:"time"`
	Event     string `json:"event"`
	RequestID string `json:"request_id,omitempty"`
	Item      string `json:"item,omitempty"`
	Mode      string `json:"mode,omitempty"`
	Requester string `json:"requester,omitempty"`
	Target    string `json:"target,omitempty"`
	Device    string `json:"device,omitempty"`
	Tenant    string `json:"tenant,omitempty"`
	Decision  string `json:"decision,omitempty"`
	Result    string `json:"result,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type AuditSink interface {
	Record(AuditEvent) error
}

type AuditLogger struct {
	mu   sync.Mutex
	file *os.File
}

func NewAuditLogger(path string) (*AuditLogger, error) {
	if path == "" {
		return nil, errors.New("audit log path is required")
	}
	dir := filepath.Dir(path)
	if dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, errors.New("cannot create audit log directory")
		}
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, errors.New("cannot open audit log")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, errors.New("cannot protect audit log")
	}
	return &AuditLogger{file: file}, nil
}

func (l *AuditLogger) Record(event AuditEvent) error {
	event.Time = time.Now().UTC().Format(time.RFC3339Nano)
	encoded, err := json.Marshal(event)
	if err != nil {
		return errors.New("cannot encode audit event")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.file.Write(append(encoded, '\n')); err != nil {
		return errors.New("cannot write audit event")
	}
	return nil
}

func (l *AuditLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

type noopAudit struct{}

func (noopAudit) Record(AuditEvent) error { return nil }
