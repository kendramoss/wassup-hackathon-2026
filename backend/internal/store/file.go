package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"
)

// File stores one JSON file per case in a directory, so cases survive a
// restart (local runs; on Cloud Run the disk is per-instance and temporary
// until Firestore replaces it, D25). Writes are atomic (temp file + rename).
type File struct {
	dir string
	mu  sync.RWMutex
}

var caseIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NewFile returns a store in dir, creating it if needed.
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: %w", err)
	}
	return &File{dir: dir}, nil
}

func (f *File) path(id string) (string, error) {
	if !caseIDPattern.MatchString(id) {
		return "", ErrNotFound
	}
	return filepath.Join(f.dir, id+".json"), nil
}

// Put implements Cases.
func (f *File) Put(_ context.Context, c Case) error {
	p, err := f.path(c.ID)
	if err != nil {
		return fmt.Errorf("store: invalid case id %q", c.ID)
	}
	b, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("store: encode %s: %w", c.ID, err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("store: write %s: %w", c.ID, err)
	}
	return os.Rename(tmp, p)
}

// Get implements Cases.
func (f *File) Get(_ context.Context, id string) (Case, error) {
	p, err := f.path(id)
	if err != nil {
		return Case{}, err
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return Case{}, ErrNotFound
	}
	if err != nil {
		return Case{}, fmt.Errorf("store: read %s: %w", id, err)
	}
	var c Case
	if err := json.Unmarshal(b, &c); err != nil {
		return Case{}, fmt.Errorf("store: decode %s: %w", id, err)
	}
	return c, nil
}

// List implements Cases.
func (f *File) List(ctx context.Context) ([]Case, error) {
	f.mu.RLock()
	files, err := filepath.Glob(filepath.Join(f.dir, "*.json"))
	f.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	out := make([]Case, 0, len(files))
	for _, p := range files {
		id := filepath.Base(p)
		c, err := f.Get(ctx, id[:len(id)-len(".json")])
		if err != nil {
			continue // skip a corrupt file rather than failing the list
		}
		out = append(out, c)
	}
	sortNewest(out)
	return out, nil
}

var _ Cases = (*File)(nil)
