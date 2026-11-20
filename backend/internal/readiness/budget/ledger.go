package budget

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const genesis = "0000000000000000000000000000000000000000000000000000000000000000"

// Record types in the attempts ledger.
const (
	TypeReserve = "reserve"
	TypeSettle  = "settle"
	TypeAck     = "ack_failures"
	TypeHalt    = "halt"
	TypeAckHalt = "ack_halt"
	// TypeExt raises the remembered external spend/reservation floor; it can
	// only rise. TypeExtReconcile is the audited exception, see
	// ReconcileExternalReserved.
	TypeExt          = "ext"
	TypeExtReconcile = "ext_reconcile"
	// TypeBind records public, non-secret hashes the run is bound to.
	TypeBind = "bind"
)

// Settle outcomes. Everything except OutcomeOK and OutcomeNotSent charges the
// full reservation (or the larger billed amount on overrun).
const (
	OutcomeOK            = "ok"
	OutcomeOKUnknownUse  = "ok_unknown_usage"
	OutcomeFailure       = "failure"
	OutcomeTimeout       = "timeout"
	OutcomeCanceled      = "canceled"
	OutcomeNotSent       = "not_sent" // the wrapped transport was never invoked: certain, charge 0
	OutcomeCrashRecovery = "crash_recovered"
	OutcomeOverrun       = "overrun"
	OutcomeBadModel      = "unexpected_model"
)

// Record is one ledger line. It never carries keys, headers or bodies; only a
// prompt hash.
type Record struct {
	Seq             int64             `json:"seq"`
	Prev            string            `json:"prev"`
	Type            string            `json:"type"`
	AttemptID       string            `json:"attempt_id,omitempty"`
	UTC             string            `json:"utc"`
	RunID           string            `json:"run_id,omitempty"`
	CaseID          string            `json:"case_id,omitempty"`
	Alias           string            `json:"alias,omitempty"`
	PriceSource     string            `json:"price_source,omitempty"`
	PriceVerifiedAt string            `json:"price_verified_at,omitempty"`
	ReservedNanos   int64             `json:"reserved_nanos,omitempty"`
	InTokensUpper   int64             `json:"in_tokens_upper,omitempty"`
	MaxOut          int64             `json:"max_out,omitempty"`
	PromptSHA       string            `json:"prompt_sha256,omitempty"`
	Outcome         string            `json:"outcome,omitempty"`
	ChargedNanos    int64             `json:"charged_nanos,omitempty"`
	InTok           int64             `json:"in_tok,omitempty"`
	OutTok          int64             `json:"out_tok,omitempty"`
	HTTPStatus      int               `json:"http_status,omitempty"`
	ErrorClass      string            `json:"error_class,omitempty"`
	Note            string            `json:"note,omitempty"`
	ExtSpentNanos   int64             `json:"ext_spent_nanos,omitempty"`
	ExtResNanos     int64             `json:"ext_res_nanos,omitempty"`
	Bind            map[string]string `json:"bind,omitempty"`
}

// notePending marks an ext record written while a lowered external
// reservation awaits reconciliation: the observed spend rise is remembered
// (floor) but the conservation baseline does not advance.
const notePending = "pending_reserve_lowered"

// Status is the state recomputed from the chain.
type Status struct {
	Records       int64
	SpentNanos    int64 // chain spend only
	OpenNanos     int64
	OpenAttempts  int
	AttemptsRaw   int64
	FailuresRaw   int64
	FailureStreak int
	Halted        bool
	HaltNote      string
	ExtSpent      int64 // remembered floor of spend recorded by other payers
	ExtRes        int64 // remembered floor of external reservations
	ExtBase       int64 // external spend at the last consistent observation (conservation baseline)
	Bound         map[string]string
	CaseAttempts  map[string]int
	CaseNanos     map[string]int64
}

type openRes struct {
	nanos  int64
	caseID string
}

type chainState struct {
	Status
	lastHash string
	seq      int64
	open     map[string]openRes
	lastType string
	// The tail since the last reserve record is what a crash between a
	// chain append and the shared-file rewrite can leave unsynced.
	tailCharged, tailReleased, tailFailures int64
}

func newChain() *chainState {
	return &chainState{lastHash: genesis, open: map[string]openRes{},
		Status: Status{CaseAttempts: map[string]int{}, CaseNanos: map[string]int64{}}}
}

func (c *chainState) apply(r Record, raw []byte) error {
	if r.Seq != c.seq+1 {
		return fmt.Errorf("%w: seq %d after %d", ErrCorrupt, r.Seq, c.seq)
	}
	if r.Prev != c.lastHash {
		return fmt.Errorf("%w: chain break at seq %d", ErrCorrupt, r.Seq)
	}
	switch r.Type {
	case TypeReserve:
		if r.AttemptID == "" || r.ReservedNanos <= 0 {
			return fmt.Errorf("%w: bad reserve at seq %d", ErrCorrupt, r.Seq)
		}
		if _, dup := c.open[r.AttemptID]; dup {
			return fmt.Errorf("%w: duplicate attempt %s", ErrCorrupt, r.AttemptID)
		}
		c.open[r.AttemptID] = openRes{r.ReservedNanos, r.CaseID}
		c.OpenNanos += r.ReservedNanos
		c.AttemptsRaw++
		c.CaseAttempts[r.CaseID]++
		c.CaseNanos[r.CaseID] += r.ReservedNanos
		c.tailCharged, c.tailReleased, c.tailFailures = 0, 0, 0
	case TypeSettle:
		res, ok := c.open[r.AttemptID]
		if !ok {
			return fmt.Errorf("%w: settle without reserve %s", ErrCorrupt, r.AttemptID)
		}
		over := r.Outcome == OutcomeOverrun || r.Outcome == OutcomeBadModel
		if r.ChargedNanos < 0 || (!over && r.ChargedNanos > res.nanos) {
			return fmt.Errorf("%w: bad charge at seq %d", ErrCorrupt, r.Seq)
		}
		delete(c.open, r.AttemptID)
		c.OpenNanos -= res.nanos
		c.SpentNanos += r.ChargedNanos
		c.CaseNanos[res.caseID] += r.ChargedNanos - res.nanos
		c.tailCharged += r.ChargedNanos
		c.tailReleased += res.nanos
		if r.Outcome == OutcomeOK {
			c.FailureStreak = 0
		} else if r.Outcome != OutcomeNotSent { // known unsent: neither a success nor a failure
			c.FailureStreak++
			c.FailuresRaw++
			c.tailFailures++
		}
	case TypeAck:
		c.FailureStreak = 0
	case TypeExt:
		if r.ExtSpentNanos < c.ExtSpent || r.ExtResNanos < c.ExtRes {
			return fmt.Errorf("%w: external floor lowered at seq %d", ErrCorrupt, r.Seq)
		}
		c.ExtSpent, c.ExtRes = r.ExtSpentNanos, r.ExtResNanos
		if r.Note != notePending {
			c.ExtBase = r.ExtSpentNanos
		}
	case TypeBind:
		if len(r.Bind) == 0 {
			return fmt.Errorf("%w: empty bind at seq %d", ErrCorrupt, r.Seq)
		}
		if c.Bound == nil {
			c.Bound = map[string]string{}
		}
		for k, v := range r.Bind {
			if old, ok := c.Bound[k]; ok && old != v {
				return fmt.Errorf("%w: %s rebound to a different value at seq %d", ErrCorrupt, k, r.Seq)
			}
			c.Bound[k] = v
		}
	case TypeExtReconcile:
		if len(r.Note) < 40 || r.ExtSpentNanos < c.ExtSpent || r.ExtSpentNanos+r.ExtResNanos < c.ExtBase+c.ExtRes {
			return fmt.Errorf("%w: unaudited or budget-returning reconcile at seq %d", ErrCorrupt, r.Seq)
		}
		c.ExtSpent, c.ExtRes, c.ExtBase = r.ExtSpentNanos, r.ExtResNanos, r.ExtSpentNanos
	case TypeHalt:
		c.Halted, c.HaltNote = true, r.Note
	case TypeAckHalt:
		c.Halted, c.HaltNote = false, ""
	default:
		return fmt.Errorf("%w: unknown type %q", ErrCorrupt, r.Type)
	}
	c.lastType = r.Type
	c.seq = r.Seq
	c.Records = r.Seq
	h := sha256.Sum256(raw)
	c.lastHash = hex.EncodeToString(h[:])
	c.OpenAttempts = len(c.open)
	return nil
}

// replay validates the entire file. A final line without a newline, an
// unparsable line or any chain error is corruption: the run stops.
func replay(b []byte) (*chainState, error) {
	c := newChain()
	if len(b) == 0 {
		return c, nil
	}
	if b[len(b)-1] != '\n' {
		return nil, fmt.Errorf("%w: truncated final line", ErrCorrupt)
	}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		line := append([]byte(nil), sc.Bytes()...)
		var r Record
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&r); err != nil {
			return nil, fmt.Errorf("%w: unparsable line after seq %d", ErrCorrupt, c.seq)
		}
		if err := c.apply(r, line); err != nil {
			return nil, err
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	return c, nil
}

// fileExternalSpend returns the external spend the shared file implies. A
// file at or above chain+floor is taken at face value (any excess is
// external, the conservative reading). Only right after a crash window
// (tolerate) may it sit below the chain, by at most the unsynced tail; any
// other shortfall is a conflict.
func fileExternalSpend(c *chainState, s *shared, tolerate bool) (int64, bool) {
	ext := s.spentNanos - c.SpentNanos
	if ext >= c.ExtSpent {
		return ext, true
	}
	if tolerate && s.spentNanos >= c.SpentNanos-c.tailCharged+c.ExtSpent {
		return c.ExtSpent, true // lagging by the unsynced tail, nothing external
	}
	return ext, false
}

// lag is how far the shared file may legitimately trail the chain right after
// a crash between a chain append and the file rewrite: the settles since the
// last reserve (money and failures) and one unsynced reserve (attempts).
func (c *chainState) lag() (spent, failures, attempts int64) {
	return c.tailCharged, c.tailFailures, 1
}

// shared is the coordinator's SHAREDBUDGET.json, read as a generic object so
// unknown fields survive a rewrite. The attempts ledger is the authoritative
// record; this file is cross-checked and kept in step.
type shared struct {
	raw         map[string]json.RawMessage
	capNanos    int64
	spentNanos  int64
	resNanos    int64
	extResNanos int64 // optional external_reserved_usd: sub-allocation for other payers (e.g. CI smoke)
	attemptsRaw int64
	failuresRaw int64
	authorized  bool
}

func readShared(path string) (*shared, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("budget: read shared budget file: %w", err)
	}
	s := &shared{raw: map[string]json.RawMessage{}}
	if err := json.Unmarshal(b, &s.raw); err != nil {
		return nil, fmt.Errorf("budget: shared budget file: %w", err)
	}
	usdField := func(k string, required bool) (int64, error) {
		rm, ok := s.raw[k]
		if !ok {
			if required {
				return 0, fmt.Errorf("budget: shared budget file: missing %s", k)
			}
			return 0, nil
		}
		v, err := ParseDecimalNanos(string(rm), 9)
		if err != nil {
			return 0, fmt.Errorf("budget: shared budget file: bad %s", k)
		}
		return v, nil
	}
	cntField := func(k string) (int64, error) {
		n, err := strconv.ParseInt(strings.TrimSpace(string(s.raw[k])), 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("budget: shared budget file: bad %s", k)
		}
		return n, nil
	}
	if s.capNanos, err = usdField("cap_usd", true); err != nil {
		return nil, err
	}
	if s.spentNanos, err = usdField("spent_usd", true); err != nil {
		return nil, err
	}
	if s.resNanos, err = usdField("reserved_usd", true); err != nil {
		return nil, err
	}
	if s.extResNanos, err = usdField("external_reserved_usd", false); err != nil {
		return nil, err
	}
	if s.attemptsRaw, err = cntField("attempts_raw"); err != nil {
		return nil, err
	}
	if s.failuresRaw, err = cntField("failures_raw"); err != nil {
		return nil, err
	}
	_ = json.Unmarshal(s.raw["calls_authorized"], &s.authorized) // absent/non-bool => false
	return s, nil
}

func writeShared(path string, s *shared, spent, reserved, attempts, failures int64) error {
	s.raw["spent_usd"] = json.RawMessage(FormatNanos(spent))
	s.raw["reserved_usd"] = json.RawMessage(FormatNanos(reserved))
	s.raw["attempts_raw"] = json.RawMessage(strconv.FormatInt(attempts, 10))
	s.raw["failures_raw"] = json.RawMessage(strconv.FormatInt(failures, 10))
	b, err := json.MarshalIndent(s.raw, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".sharedbudget-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err = tmp.Write(append(b, '\n')); err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// CheckLocalPath refuses locations where advisory flock and fsync cannot be
// trusted: cloud-synced folders and (on darwin) network or FUSE filesystems.
func CheckLocalPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for _, bad := range []string{"/Mobile Documents/", "/Library/CloudStorage/", "/Dropbox/", "/OneDrive"} {
		if strings.Contains(abs+"/", bad) {
			return fmt.Errorf("%w: %s", ErrNonLocalFS, bad)
		}
	}
	return checkFSType(filepath.Dir(abs))
}

// Inspect verifies the chain and the shared file without taking the lock or
// writing anything. Used by `budget status`.
func Inspect(attemptsPath, sharedPath string) (Status, int64, bool, error) {
	b, err := os.ReadFile(attemptsPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Status{}, 0, false, err
	}
	c, err := replay(b)
	if err != nil {
		return Status{}, 0, false, err
	}
	s, err := readShared(sharedPath)
	if err != nil {
		return Status{}, 0, false, err
	}
	_, lagF, lagA := c.lag()
	if _, ok := fileExternalSpend(c, s, true); !ok || s.attemptsRaw < c.AttemptsRaw-lagA || s.failuresRaw < c.FailuresRaw-lagF {
		return c.Status, s.capNanos, s.authorized, fmt.Errorf("%w: shared counters below the chain", ErrTamper)
	}
	if s.extResNanos < c.ExtRes {
		return c.Status, s.capNanos, s.authorized, ErrExtReservedLowered
	}
	return c.Status, s.capNanos, s.authorized, nil
}

// Ledger is the open, locked attempts ledger.
type Ledger struct {
	mu         sync.Mutex
	path       string
	sharedPath string
	lock, f    *os.File
	c          *chainState
	ext        extern
	poisoned   error
	now        func() time.Time
	// lagOK is true from Open until the first successful shared-file write:
	// the window in which a crash may have left the file behind the chain.
	lagOK bool
}

// extern holds counters recorded in the shared file that the chain does not
// (effective = max(chain, file)); they only rise. External money lives in the
// chain as TypeExt floors so a restart remembers it too.
type extern struct{ attempts, failures int64 }

// ErrExtReservedLowered means the external reservation in the shared file is
// below the highest value the ledger has seen. It is refused until an audited
// reconciliation (ReconcileExternalReserved).
var ErrExtReservedLowered = fmt.Errorf("%w: external reservation lowered below the remembered floor", ErrTamper)

// refreshExternalLocked is called under the lock on every Reserve and Settle.
// It re-reads nothing itself; it takes the freshly read file. External money
// and reservations are monotonic: a rise is remembered durably, a fall is a
// conflict and fails closed. Nothing is ever reset to the chain alone.
func (l *Ledger) refreshExternalLocked(s *shared) error {
	extSpent, ok := fileExternalSpend(l.c, s, l.lagOK)
	if !ok {
		return fmt.Errorf("%w: spent_usd below the ledger plus remembered external spend", ErrTamper)
	}
	lagF, lagA := int64(0), int64(0)
	if l.lagOK {
		_, lagF, lagA = l.c.lag()
	}
	if s.attemptsRaw < l.c.AttemptsRaw+l.ext.attempts-lagA || s.failuresRaw < l.c.FailuresRaw+l.ext.failures-lagF {
		return fmt.Errorf("%w: shared counters edited downward", ErrTamper)
	}
	if d := s.attemptsRaw - l.c.AttemptsRaw; d > l.ext.attempts {
		l.ext.attempts = d
	}
	if d := s.failuresRaw - l.c.FailuresRaw; d > l.ext.failures {
		l.ext.failures = d
	}
	lowered := s.extResNanos < l.c.ExtRes
	if extSpent > l.c.ExtSpent || s.extResNanos > l.c.ExtRes {
		// Remember what the operator raised first, under the lock and in the
		// chain, even when a lowered reservation is also pending. While one
		// is pending the conservation baseline does not advance.
		rec := Record{Type: TypeExt, ExtSpentNanos: extSpent, ExtResNanos: maxI(s.extResNanos, l.c.ExtRes)}
		if lowered {
			rec.Note = notePending
		}
		if err := l.appendLocked(rec); err != nil {
			return err
		}
	}
	if lowered {
		return ErrExtReservedLowered
	}
	return nil
}

// ReconcileExternalReserved is the only way to accept a lowered
// external_reserved_usd. It needs a note of at least 40 characters naming
// the evidence, and it refuses unless the spent_usd in the file rose by at
// least the amount the reservation fell (money moved from reserved to spent,
// not returned to the budget). It is deliberately not exposed on the CLI.
func (l *Ledger) ReconcileExternalReserved(note string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(strings.TrimSpace(note)) < 40 {
		return errors.New("budget: reconcile note must cite the evidence (40+ characters)")
	}
	s, err := readShared(l.sharedPath)
	if err != nil {
		return err
	}
	if s.extResNanos >= l.c.ExtRes {
		return errors.New("budget: nothing to reconcile")
	}
	extSpent, ok := fileExternalSpend(l.c, s, l.lagOK)
	if !ok {
		return fmt.Errorf("%w: spent_usd below the ledger plus remembered external spend", ErrTamper)
	}
	if extSpent < l.c.ExtSpent || extSpent-l.c.ExtBase < l.c.ExtRes-s.extResNanos {
		return fmt.Errorf("%w: reservation fell by more than spend rose; refusing to return budget", ErrTamper)
	}
	if err := l.appendLocked(Record{Type: TypeExtReconcile, Note: note, ExtSpentNanos: extSpent, ExtResNanos: s.extResNanos}); err != nil {
		return err
	}
	// Heal the shared file (it may trail the chain after a crash); this never
	// writes external_reserved_usd, so the operator's value is left alone.
	return l.syncSharedLocked(s)
}

// Bind records public, non-secret hashes (for example pricing_proof_sha256,
// config_sha256, guard_commit, run_ends_by, and later the screening, gold,
// leakage and input hashes) so the chain shows what a run was bound to. A key
// already bound to a different value is refused: one ledger, one frozen run.
func (l *Ledger) Bind(kv map[string]string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(kv) == 0 {
		return errors.New("budget: nothing to bind")
	}
	for k, v := range kv {
		if k == "" || v == "" || strings.ContainsAny(k+v, "\n\r") || len(v) > 200 {
			return errors.New("budget: bad bind entry")
		}
		if strings.HasSuffix(k, "_sha256") && len(v) != 64 {
			return fmt.Errorf("budget: %s must be a 64-hex sha256", k)
		}
		if old, ok := l.c.Bound[k]; ok && old != v {
			return fmt.Errorf("budget: %s is already bound to a different value; start a new ledger for a new frozen run", k)
		}
	}
	return l.appendLocked(Record{Type: TypeBind, Bind: kv})
}

// Bound returns a copy of the bound hashes.
func (l *Ledger) Bound() map[string]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := map[string]string{}
	for k, v := range l.c.Bound {
		out[k] = v
	}
	return out
}

// Open takes the exclusive process lock (non-blocking: a second process fails
// with ErrLocked), validates the whole chain, cross-checks the shared file,
// and converts every open reservation left by a dead process into a full
// charge plus a sticky halt that a human must acknowledge after reconciling
// against the gateway. It never trusts a cached sum.
func Open(attemptsPath, sharedPath string) (*Ledger, error) {
	if err := CheckLocalPath(attemptsPath); err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(attemptsPath+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lf.Close()
		return nil, ErrLocked
	}
	fail := func(err error) (*Ledger, error) { lf.Close(); return nil, err }
	f, err := os.OpenFile(attemptsPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fail(err)
	}
	failf := func(err error) (*Ledger, error) { f.Close(); return fail(err) }
	if err := syncDir(filepath.Dir(attemptsPath)); err != nil {
		return failf(err)
	}
	b, err := os.ReadFile(attemptsPath)
	if err != nil {
		return failf(err)
	}
	c, err := replay(b)
	if err != nil {
		return failf(err)
	}
	s, err := readShared(sharedPath)
	if err != nil {
		return failf(err)
	}
	l := &Ledger{path: attemptsPath, sharedPath: sharedPath, lock: lf, f: f, c: c, now: time.Now}
	// A crash may have left the file behind the chain by the unsynced tail
	// (lag): that is healed below, never a hand edit.
	l.lagOK = true
	if s.resNanos > c.OpenNanos+c.tailReleased {
		return failf(fmt.Errorf("%w: shared file holds a reservation the chain does not", ErrTamper))
	}
	// A lowered external reservation does not stop Open (so the audited
	// reconcile can run) but Reserve refuses until then, and the file is not
	// rewritten meanwhile so the operator's edit survives.
	pending := false
	if err := l.refreshExternalLocked(s); err != nil {
		if !errors.Is(err, ErrExtReservedLowered) {
			return failf(err)
		}
		pending = true
	}
	recovered := 0
	for id, res := range c.open {
		if err := l.appendLocked(Record{Type: TypeSettle, AttemptID: id, Outcome: OutcomeCrashRecovery, ChargedNanos: res.nanos, ErrorClass: "open_at_start"}); err != nil {
			return failf(err)
		}
		recovered++
	}
	if recovered > 0 {
		if err := l.appendLocked(Record{Type: TypeHalt, Note: fmt.Sprintf("%d attempt(s) were open at start; spend is unreconciled", recovered)}); err != nil {
			return failf(err)
		}
	}
	if !pending {
		if err := l.syncSharedLocked(s); err != nil {
			return failf(err)
		}
	}
	return l, nil
}

// Close releases the lock and files.
func (l *Ledger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e1 := l.f.Close()
	e2 := l.lock.Close()
	if e1 != nil {
		return e1
	}
	return e2
}

// SetClock replaces the clock (tests).
func (l *Ledger) SetClock(now func() time.Time) { l.now = now }

func (l *Ledger) appendLocked(r Record) error {
	r.Seq = l.c.seq + 1
	r.Prev = l.c.lastHash
	if r.UTC == "" {
		r.UTC = l.now().UTC().Format(time.RFC3339Nano)
	}
	line, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err := l.c.apply(r, line); err != nil {
		return err
	}
	if _, err := l.f.Write(append(line, '\n')); err != nil {
		l.poisoned = fmt.Errorf("%w: append failed: %v", ErrPoisoned, err)
		return l.poisoned
	}
	if err := l.f.Sync(); err != nil {
		l.poisoned = fmt.Errorf("%w: fsync failed: %v", ErrPoisoned, err)
		return l.poisoned
	}
	return nil
}

func (l *Ledger) syncSharedLocked(s *shared) error {
	if err := l.syncSharedWrite(s); err != nil {
		return err
	}
	l.lagOK = false
	return nil
}

func (l *Ledger) syncSharedWrite(s *shared) error {
	return writeShared(l.sharedPath, s, l.c.SpentNanos+l.c.ExtSpent, l.c.OpenNanos,
		l.c.AttemptsRaw+l.ext.attempts, l.c.FailuresRaw+l.ext.failures)
}

// Snapshot returns the current chain status.
func (l *Ledger) Snapshot() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	st := l.c.Status
	st.CaseAttempts = map[string]int{}
	for k, v := range l.c.CaseAttempts {
		st.CaseAttempts[k] = v
	}
	st.CaseNanos = map[string]int64{}
	for k, v := range l.c.CaseNanos {
		st.CaseNanos[k] = v
	}
	return st
}

// ReserveReq describes one reservation and the limits to enforce.
type ReserveReq struct {
	RunID, CaseID, Alias       string
	PriceSource, PriceVerified string
	Nanos, InUpper, MaxOut     int64
	PromptSHA                  string
	MaxCaseAttempts            int
	MaxCaseNanos               int64
	MaxFailureStreak           int
}

// Reservation is an open reservation handle.
type Reservation struct {
	ID    string
	Nanos int64
}

// Reserve re-reads the shared file (authorization, downward edits, cap),
// checks every limit, then appends and fsyncs the reservation. Only after it
// returns nil may the caller send the request.
func (l *Ledger) Reserve(q ReserveReq) (*Reservation, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.poisoned != nil {
		return nil, l.poisoned
	}
	if l.c.Halted {
		return nil, fmt.Errorf("%w: %s", ErrHalted, l.c.HaltNote)
	}
	s, err := readShared(l.sharedPath)
	if err != nil {
		return nil, err
	}
	if !s.authorized {
		return nil, ErrUnauthorized
	}
	if err := l.refreshExternalLocked(s); err != nil {
		return nil, err
	}
	if s.resNanos > l.c.OpenNanos {
		return nil, fmt.Errorf("%w: shared reservation above the chain", ErrTamper)
	}
	cp := s.capNanos
	if cp > HardCapNanos {
		cp = HardCapNanos
	}
	if q.Nanos <= 0 {
		return nil, fmt.Errorf("%w: non-positive reservation", ErrBudget)
	}
	committed := l.c.SpentNanos + l.c.ExtSpent + l.c.OpenNanos + l.c.ExtRes
	if committed < 0 || committed+q.Nanos > cp || committed+q.Nanos < 0 {
		return nil, ErrBudget
	}
	if (q.MaxCaseAttempts > 0 && l.c.CaseAttempts[q.CaseID] >= q.MaxCaseAttempts) ||
		(q.MaxCaseNanos > 0 && l.c.CaseNanos[q.CaseID]+q.Nanos > q.MaxCaseNanos) {
		return nil, ErrCaseCap
	}
	if q.MaxFailureStreak > 0 && l.c.FailureStreak >= q.MaxFailureStreak {
		return nil, ErrFailureStreak
	}
	id := fmt.Sprintf("a%06d", l.c.seq+1)
	rec := Record{Type: TypeReserve, AttemptID: id, RunID: q.RunID, CaseID: q.CaseID, Alias: q.Alias,
		PriceSource: q.PriceSource, PriceVerifiedAt: q.PriceVerified, ReservedNanos: q.Nanos,
		InTokensUpper: q.InUpper, MaxOut: q.MaxOut, PromptSHA: q.PromptSHA}
	if err := l.appendLocked(rec); err != nil {
		return nil, err
	}
	if err := l.syncSharedLocked(s); err != nil {
		l.poisoned = fmt.Errorf("%w: shared file write failed: %v", ErrPoisoned, err)
		return nil, l.poisoned
	}
	return &Reservation{ID: id, Nanos: q.Nanos}, nil
}

// SettleReq describes how an attempt ended.
type SettleReq struct {
	Outcome    string
	Charged    int64
	InTok      int64
	OutTok     int64
	HTTPStatus int
	ErrorClass string
}

// Settle closes a reservation. It takes no context: a settle write must never
// be cancelled. A charge above the reservation is accepted only for overrun
// and unexpected-model outcomes (the provider billed past the bound).
func (l *Ledger) Settle(r *Reservation, q SettleReq) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	over := q.Outcome == OutcomeOverrun || q.Outcome == OutcomeBadModel
	if !over && q.Charged > r.Nanos {
		q.Charged = r.Nanos
	}
	// Read and reconcile the shared file BEFORE appending: the file still
	// reflects the previous chain state, so external money is measured
	// against it. The settle itself is recorded regardless (the chain is the
	// truth); a conflicting edit then fails closed and is never overwritten.
	s, rerr := readShared(l.sharedPath)
	var terr error
	if rerr == nil {
		terr = l.refreshExternalLocked(s)
	}
	if err := l.appendLocked(Record{Type: TypeSettle, AttemptID: r.ID, Outcome: q.Outcome, ChargedNanos: q.Charged,
		InTok: q.InTok, OutTok: q.OutTok, HTTPStatus: q.HTTPStatus, ErrorClass: q.ErrorClass}); err != nil {
		return err
	}
	switch {
	case rerr != nil:
		l.poisoned = fmt.Errorf("%w: shared file read failed: %v", ErrPoisoned, rerr)
		return l.poisoned
	case terr != nil:
		l.poisoned = fmt.Errorf("%w: %v", ErrPoisoned, terr)
		return l.poisoned
	}
	if err := l.syncSharedLocked(s); err != nil {
		l.poisoned = fmt.Errorf("%w: shared file write failed: %v", ErrPoisoned, err)
		return l.poisoned
	}
	return nil
}

// Halt records a sticky halt that survives restarts until AckHalt.
func (l *Ledger) Halt(note string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendLocked(Record{Type: TypeHalt, Note: note})
}

// AckHalt records a human decision, made after reconciling against the
// gateway, to clear a sticky halt.
func (l *Ledger) AckHalt(note string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendLocked(Record{Type: TypeAckHalt, Note: note})
}

// AckFailures records a human decision to clear the consecutive-failure stop.
func (l *Ledger) AckFailures(note string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.appendLocked(Record{Type: TypeAck, Note: note})
}
