package audit

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// File is a local Store: one append-only JSON-lines file per tenant plus a
// head file and, after a purge, a checkpoint file.
//
// It is single-process. The mutex serialises one process; two processes
// sharing a directory can interleave appends and are NOT supported (no file
// lock is taken and none is claimed). It is for local runs; deployments use
// Firestore. The head file lets Verify notice a truncated tail, but an
// attacker with write access to the directory can rewrite the events, the head
// and the checkpoint together: this is tamper evidence, not immutability.
type File struct {
	dir   string
	mu    sync.Mutex
	heads map[string]Head
}

type fileHead struct {
	Tenant string `json:"tenant"`
	Head
}

// NewFile returns a store in dir, creating it if needed.
func NewFile(dir string) (*File, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("audit: %w", err)
	}
	return &File{dir: dir, heads: map[string]Head{}}, nil
}

func (f *File) base(tenant string) string {
	sum := sha256.Sum256([]byte(tenant))
	return filepath.Join(f.dir, hex.EncodeToString(sum[:16]))
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	fh, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := fh.Write(b); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Sync(); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readEvents parses the tenant file. A trailing fragment without a newline (a
// crash mid-write) is dropped and the file truncated to the last whole line.
func (f *File) readEvents(tenant string) ([]Event, error) {
	p := f.base(tenant) + ".jsonl"
	b, err := os.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit: read: %w", err)
	}
	if i := bytes.LastIndexByte(b, '\n'); i != len(b)-1 {
		b = b[:i+1]
		if err := os.Truncate(p, int64(len(b))); err != nil {
			return nil, fmt.Errorf("audit: repair: %w", err)
		}
	}
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("audit: a stored line is not an event: %w", err)
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

func (f *File) readCheckpoint(tenant string) (*Checkpoint, error) {
	b, err := os.ReadFile(f.base(tenant) + ".cp.json")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("audit: read checkpoint: %w", err)
	}
	var cp Checkpoint
	if err := json.Unmarshal(b, &cp); err != nil {
		return nil, fmt.Errorf("audit: checkpoint: %w", err)
	}
	return &cp, nil
}

// head loads (once per process) and returns the tenant's head. The head file
// is written after the event, so a crash between the two leaves the event one
// ahead of the file: that single step is healed forward when its prev hash
// matches. Anything else (an event missing from the end) leaves the file's
// head in place, and Verify reports the mismatch.
func (f *File) head(tenant string) (Head, error) {
	if h, ok := f.heads[tenant]; ok {
		return h, nil
	}
	var h Head
	var haveFile bool
	if b, err := os.ReadFile(f.base(tenant) + ".head.json"); err == nil {
		var fh fileHead
		if err := json.Unmarshal(b, &fh); err != nil {
			return Head{}, fmt.Errorf("audit: head: %w", err)
		}
		h, haveFile = fh.Head, true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return Head{}, fmt.Errorf("audit: head: %w", err)
	}
	evs, err := f.readEvents(tenant)
	if err != nil {
		return Head{}, err
	}
	if n := len(evs); n > 0 {
		last := evs[n-1]
		switch {
		case !haveFile:
			h = HeadOf(last)
		case last.Seq == h.Seq+1 && last.PrevHash == h.Hash:
			h = HeadOf(last)
		}
	}
	f.heads[tenant] = h
	return h, nil
}

func (f *File) writeHead(tenant string, h Head) error {
	b, _ := json.Marshal(fileHead{Tenant: tenant, Head: h})
	return writeAtomic(f.base(tenant)+".head.json", b)
}

// Append implements Store.
func (f *File) Append(_ context.Context, tenant string, seal func(Head) (Event, error)) (Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, err := f.head(tenant)
	if err != nil {
		return Event{}, err
	}
	e, err := seal(h)
	if err != nil {
		return Event{}, err
	}
	if e.Tenant != tenant || e.Seq != h.Seq+1 {
		return Event{}, ErrInvalid
	}
	line, err := json.Marshal(e)
	if err != nil {
		return Event{}, err
	}
	fh, err := os.OpenFile(f.base(tenant)+".jsonl", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return Event{}, fmt.Errorf("audit: append: %w", err)
	}
	_, werr := fh.Write(append(line, '\n'))
	if werr == nil {
		werr = fh.Sync()
	}
	if cerr := fh.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		delete(f.heads, tenant) // reload from disk next time
		return Event{}, fmt.Errorf("audit: append: %w", werr)
	}
	nh := HeadOf(e)
	if err := f.writeHead(tenant, nh); err != nil {
		delete(f.heads, tenant)
		return Event{}, fmt.Errorf("audit: head: %w", err)
	}
	f.heads[tenant] = nh
	return e, nil
}

// Range implements Store.
func (f *File) Range(_ context.Context, tenant string, r Range) ([]Event, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	evs, err := f.readEvents(tenant)
	if err != nil {
		return nil, err
	}
	var out []Event
	if r.Desc {
		for i := len(evs) - 1; i >= 0 && len(out) < r.Limit; i-- {
			if r.Cursor == 0 || evs[i].Seq < r.Cursor {
				out = append(out, evs[i])
			}
		}
		return out, nil
	}
	for _, e := range evs {
		if e.Seq > r.Cursor {
			out = append(out, e)
			if len(out) >= r.Limit {
				break
			}
		}
	}
	return out, nil
}

// State implements Store.
func (f *File) State(_ context.Context, tenant string) (Head, *Checkpoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	h, err := f.head(tenant)
	if err != nil {
		return Head{}, nil, err
	}
	cp, err := f.readCheckpoint(tenant)
	return h, cp, err
}

// Purge implements Store. The checkpoint is written first, then the events
// are rewritten without the purged prefix (temp file + rename). A crash
// between the two leaves events the checkpoint already covers, which Verify
// skips.
func (f *File) Purge(_ context.Context, tenant string, cutoff, now time.Time, limit int) (PurgeResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	evs, err := f.readEvents(tenant)
	if err != nil {
		return PurgeResult{}, err
	}
	cp, err := f.readCheckpoint(tenant)
	if err != nil {
		return PurgeResult{}, err
	}
	if limit <= 0 {
		limit = DefaultPurgeChunk
	}
	n := 0
	for n < len(evs) && n < limit && evs[n].Time.Before(cutoff) {
		n++
	}
	if n == 0 {
		return PurgeResult{Checkpoint: cp}, nil
	}
	last := evs[n-1]
	ncp := &Checkpoint{ThroughSeq: last.Seq, ThroughHash: last.Hash, ThroughTime: last.Time, PurgedAt: now.UTC()}
	b, _ := json.Marshal(ncp)
	if err := writeAtomic(f.base(tenant)+".cp.json", b); err != nil {
		return PurgeResult{}, fmt.Errorf("audit: checkpoint: %w", err)
	}
	var buf bytes.Buffer
	for _, e := range evs[n:] {
		line, _ := json.Marshal(e)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := writeAtomic(f.base(tenant)+".jsonl", buf.Bytes()); err != nil {
		return PurgeResult{}, fmt.Errorf("audit: purge: %w", err)
	}
	return PurgeResult{Deleted: n, Checkpoint: ncp}, nil
}

// Tenants implements Store.
func (f *File) Tenants(_ context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	files, err := filepath.Glob(filepath.Join(f.dir, "*.head.json"))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var fh fileHead
		if json.Unmarshal(b, &fh) == nil && ValidTenant(fh.Tenant) {
			out = append(out, fh.Tenant)
		}
	}
	sort.Strings(out)
	return out, nil
}

var _ Store = (*File)(nil)
