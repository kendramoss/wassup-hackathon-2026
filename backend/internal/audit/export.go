package audit

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"
)

// Export of a tenant's trail (#871). The JSONL form is the verifiable one:
//
//	line 1   {"type":"header", ...}   the head and retention checkpoint read
//	                                   before any event, so the export is a
//	                                   snapshot that later appends cannot extend
//	lines    one Event per line, oldest first, exactly as stored (with
//	         prev_hash and hash)
//	last     {"type":"summary", ...}  the event count and the SHA-256 of every
//	                                   byte before the summary line
//
// VerifyExport re-checks a JSONL export offline with the same chain rules as
// GET /v1/audit/verify. Like the trail itself this is tamper evidence, not a
// signature: whoever can rewrite the store can rewrite a consistent export.

// ExportFormatVersion is the version a header carries.
const ExportFormatVersion = 1

// ExportHeader is the first JSONL line.
type ExportHeader struct {
	Type          string      `json:"type"` // "header"
	FormatVersion int         `json:"format_version"`
	Tenant        string      `json:"tenant"`
	ExportedAt    time.Time   `json:"exported_at"`
	Head          Head        `json:"head"`
	Checkpoint    *Checkpoint `json:"checkpoint,omitempty"`
	RetentionDays int         `json:"retention_days"`
	// From and To, when set, narrow the export by event time. A narrowed
	// export is not a whole chain: VerifyExport then checks each event's hash
	// and the links between consecutive events only.
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
}

// ExportSummary is the last JSONL line.
type ExportSummary struct {
	Type   string `json:"type"` // "summary"
	Events int    `json:"events"`
	// SHA256 is the hex SHA-256 of every byte of the export before this line.
	SHA256 string `json:"sha256"`
	// Complete is false when the export stopped at MaxExportEvents.
	Complete bool `json:"complete"`
}

// MaxExportEvents bounds one export. A tenant with more events exports in
// time windows (from, to).
const MaxExportEvents = 200_000

// ErrExportRestart means a retention purge removed events the export still
// needed. Start the export again.
var ErrExportRestart = errors.New("audit: a retention purge ran during the export; start it again")

// ExportSnapshot reads tenant's head and retention checkpoint: the header of
// an export, fixed before any event is read.
func (s *Service) ExportSnapshot(ctx context.Context, tenant string) (ExportHeader, error) {
	if !ValidTenant(tenant) {
		return ExportHeader{}, ErrBadQuery
	}
	head, cp, err := s.store.State(ctx, tenant)
	if err != nil {
		return ExportHeader{}, err
	}
	if head.Hash == "" {
		head.Hash = Genesis
	}
	return ExportHeader{Type: "header", FormatVersion: ExportFormatVersion, Tenant: tenant, ExportedAt: s.opt.Now().UTC(),
		Head: head, Checkpoint: cp, RetentionDays: int(s.opt.Retention.Hours() / 24)}, nil
}

// ExportEvents feeds emit the events of h's snapshot, oldest first, that fall
// in h.From..h.To. It returns the number emitted and whether all were.
func (s *Service) ExportEvents(ctx context.Context, h ExportHeader, max int, emit func(Event) error) (int, bool, error) {
	if max <= 0 {
		max = MaxExportEvents
	}
	start, _ := anchorFor(h.Checkpoint)
	pos, n, first := start.Seq, 0, true
	for pos < h.Head.Seq {
		evs, err := s.store.Range(ctx, h.Tenant, Range{Cursor: pos, Limit: 500})
		if err != nil {
			return n, false, err
		}
		if len(evs) == 0 {
			break
		}
		if first && evs[0].Seq != start.Seq+1 {
			return n, false, ErrExportRestart
		}
		first = false
		for _, e := range evs {
			if e.Seq > h.Head.Seq {
				return n, true, nil
			}
			pos = e.Seq
			if (h.From != nil && e.Time.Before(*h.From)) || (h.To != nil && !e.Time.Before(*h.To)) {
				continue
			}
			if n == max {
				return n, false, nil
			}
			if err := emit(e); err != nil {
				return n, false, err
			}
			n++
		}
	}
	return n, true, nil
}

// ExportWriter writes the JSONL form and keeps the running digest.
type ExportWriter struct {
	w   *bufio.Writer
	sum hash.Hash
	n   int
}

// NewExportWriter writes to w.
func NewExportWriter(w io.Writer) *ExportWriter {
	return &ExportWriter{w: bufio.NewWriter(w), sum: sha256.New()}
}

func (x *ExportWriter) line(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	x.sum.Write(b)
	_, err = x.w.Write(b)
	return err
}

// Header writes the header line.
func (x *ExportWriter) Header(h ExportHeader) error { return x.line(h) }

// Event writes one event line.
func (x *ExportWriter) Event(e Event) error { x.n++; return x.line(e) }

// Summary writes the summary line and flushes.
func (x *ExportWriter) Summary(complete bool) (ExportSummary, error) {
	s := ExportSummary{Type: "summary", Events: x.n, SHA256: hex.EncodeToString(x.sum.Sum(nil)), Complete: complete}
	b, err := json.Marshal(s)
	if err != nil {
		return s, err
	}
	if _, err := x.w.Write(append(b, '\n')); err != nil {
		return s, err
	}
	return s, x.w.Flush()
}

// Flush flushes buffered lines (for streaming).
func (x *ExportWriter) Flush() error { return x.w.Flush() }

// ExportCheck is the result of VerifyExport.
type ExportCheck struct {
	Tenant   string    `json:"tenant"`
	Events   int       `json:"events"`
	Whole    bool      `json:"whole_chain"` // false: a time-narrowed or incomplete export
	DigestOK bool      `json:"digest_ok"`
	Findings []Finding `json:"findings"`
	OK       bool      `json:"ok"`
}

// VerifyExport reads a JSONL export and checks its digest and chain. A whole
// export (no time window, complete) is checked like GET /v1/audit/verify:
// from the checkpoint (or genesis) to the header's head. A narrowed one is
// checked event by event and link by link.
func VerifyExport(r io.Reader) (ExportCheck, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4<<20)
	sum := sha256.New()
	var (
		hdr     *ExportHeader
		summary *ExportSummary
		evs     []Event
	)
	for sc.Scan() {
		raw := sc.Bytes()
		if strings.TrimSpace(string(raw)) == "" {
			continue
		}
		if summary != nil {
			return ExportCheck{}, errors.New("audit export: lines after the summary")
		}
		var probe struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return ExportCheck{}, fmt.Errorf("audit export: %w", err)
		}
		switch probe.Type {
		case "header":
			if hdr != nil || len(evs) > 0 {
				return ExportCheck{}, errors.New("audit export: the header must be the first line, once")
			}
			hdr = &ExportHeader{}
			if err := json.Unmarshal(raw, hdr); err != nil {
				return ExportCheck{}, fmt.Errorf("audit export header: %w", err)
			}
		case "summary":
			summary = &ExportSummary{}
			if err := json.Unmarshal(raw, summary); err != nil {
				return ExportCheck{}, fmt.Errorf("audit export summary: %w", err)
			}
			continue
		case "":
			if hdr == nil {
				return ExportCheck{}, errors.New("audit export: an event before the header")
			}
			var e Event
			if err := json.Unmarshal(raw, &e); err != nil {
				return ExportCheck{}, fmt.Errorf("audit export event: %w", err)
			}
			evs = append(evs, e)
		default:
			return ExportCheck{}, fmt.Errorf("audit export: unknown line type %q", probe.Type)
		}
		sum.Write(raw)
		sum.Write([]byte{'\n'})
	}
	if err := sc.Err(); err != nil {
		return ExportCheck{}, err
	}
	if hdr == nil || summary == nil {
		return ExportCheck{}, errors.New("audit export: missing header or summary")
	}
	out := ExportCheck{Tenant: hdr.Tenant, Events: len(evs), Findings: []Finding{}}
	out.DigestOK = summary.Events == len(evs) && summary.SHA256 == hex.EncodeToString(sum.Sum(nil))
	out.Whole = hdr.From == nil && hdr.To == nil && summary.Complete
	if out.Whole {
		start, anchored := anchorFor(hdr.Checkpoint)
		c := chainCheck{tenant: hdr.Tenant, expect: start, anchored: anchored, fromCP: hdr.Checkpoint != nil}
		for _, e := range evs {
			c.feed(e)
		}
		c.finish(hdr.Head, hdr.Checkpoint)
		out.Findings = append(out.Findings, c.findings...)
	} else {
		for i, e := range evs {
			if e.Tenant != hdr.Tenant {
				out.Findings = append(out.Findings, Finding{Seq: e.Seq, Kind: FindingTenant})
			}
			if e.Hash != e.ComputeHash() {
				out.Findings = append(out.Findings, Finding{Seq: e.Seq, Kind: FindingHash})
			}
			if i > 0 {
				p := evs[i-1]
				switch {
				case e.Seq <= p.Seq:
					out.Findings = append(out.Findings, Finding{Seq: e.Seq, Kind: FindingReorder})
				case e.Seq == p.Seq+1 && e.PrevHash != p.Hash:
					out.Findings = append(out.Findings, Finding{Seq: e.Seq, Kind: FindingPrev})
				}
			}
			if e.Seq > hdr.Head.Seq {
				out.Findings = append(out.Findings, Finding{Seq: e.Seq, Kind: FindingHead})
			}
		}
	}
	out.OK = out.DigestOK && len(out.Findings) == 0
	return out, nil
}
