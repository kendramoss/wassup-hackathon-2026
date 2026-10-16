// Package blob is the small key-value store behind the verdict cache (D50)
// and the engine prediction cache (D54): a directory locally, Firestore on
// Cloud Run (store.FirestoreBlobs).
package blob

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Store gets and puts blobs by key.
type Store interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Put(ctx context.Context, key string, b []byte) error
}

// Dir stores each blob as <Dir>/<key>.json.
type Dir struct{ Dir string }

// Get implements Store.
func (d Dir) Get(_ context.Context, key string) ([]byte, bool, error) {
	b, err := os.ReadFile(filepath.Join(d.Dir, key+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// Put implements Store (atomically: write a temp file, then rename).
func (d Dir) Put(_ context.Context, key string, b []byte) error {
	tmp, err := os.CreateTemp(d.Dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(d.Dir, key+".json"))
}
