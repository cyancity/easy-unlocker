package broker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
)

const maxStoredValue = 4 << 20

type SecretStore interface {
	Get(context.Context, string) ([]byte, error)
}

// RBWStore is the only production storage backend in the MVP. rbw's stdout is
// captured directly into memory and stderr is discarded so a secret cannot be
// copied into Broker logs.
type RBWStore struct {
	Binary string
}

func (s RBWStore) Get(ctx context.Context, item string) ([]byte, error) {
	// rbw 会把 - 开头的位置参数当选项，先挡掉。
	if strings.HasPrefix(item, "-") {
		return nil, errors.New("invalid item name")
	}
	if s.Binary == "" {
		s.Binary = "rbw"
	}
	cmd := exec.CommandContext(ctx, s.Binary, "get", item)
	var output cappedBuffer
	output.max = maxStoredValue
	defer output.clear()
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, errors.New("rbw get failed")
	}
	if output.tooLarge {
		return nil, errors.New("rbw value is too large")
	}
	if ctx.Err() != nil {
		return nil, errors.New("rbw get timed out")
	}
	return output.bytes(), nil
}

type cappedBuffer struct {
	buf      bytes.Buffer
	max      int
	tooLarge bool
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	remaining := b.max - b.buf.Len()
	if remaining <= 0 {
		b.tooLarge = true
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = b.buf.Write(p[:remaining])
		b.tooLarge = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *cappedBuffer) bytes() []byte {
	result := make([]byte, b.buf.Len())
	copy(result, b.buf.Bytes())
	return result
}

func (b *cappedBuffer) clear() {
	value := b.buf.Bytes()
	for i := range value {
		value[i] = 0
	}
	b.buf.Reset()
}

// MapStore is deliberately available only as an explicit test/development
// backend. Production configuration should always use RBWStore.
type MapStore struct {
	mu     sync.RWMutex
	values map[string][]byte
}

func NewMapStore(values map[string][]byte) *MapStore {
	copyValues := make(map[string][]byte, len(values))
	for item, value := range values {
		copyValues[item] = append([]byte(nil), value...)
	}
	return &MapStore{values: copyValues}
}

func (s *MapStore) Get(ctx context.Context, item string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	value, ok := s.values[item]
	s.mu.RUnlock()
	if !ok {
		return nil, errors.New("item not found")
	}
	return append([]byte(nil), value...), nil
}
