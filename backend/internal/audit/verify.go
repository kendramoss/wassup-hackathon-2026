package audit

import "time"

// Finding kinds reported by verification.
const (
	FindingHash         = "hash_mismatch"       // an event's stored hash is not the hash of its content
	FindingPrev         = "prev_mismatch"       // an event does not chain to its predecessor
	FindingGap          = "seq_gap"             // events are missing from the middle
	FindingReorder      = "seq_reorder"         // sequence went backwards or repeated
	FindingTime         = "time_regression"     // time went backwards inside the tenant
	FindingHead         = "head_mismatch"       // the newest event is not the stored head (tail removed or head edited)
	FindingCheckpoint   = "checkpoint_mismatch" // the retained suffix does not continue from the retention checkpoint
	FindingUnanchored   = "prefix_unanchored"   // the oldest events are missing with no retention checkpoint covering them
	FindingTenant       = "tenant_mismatch"     // an event of another tenant sits in this chain
	FindingInvalidEvent = "invalid_event"       // a stored event breaks the field rules
)

// Finding is one verification problem.
type Finding struct {
	Seq  uint64 `json:"seq"`
	Kind string `json:"kind"`
}

// maxFindings bounds a verification result.
const maxFindings = 100

// chainCheck verifies events fed in stored order against an anchor.
type chainCheck struct {
	tenant   string
	expect   Head
	anchored bool // false: the chain is expected to start at seq 1 with no checkpoint
	fromCP   bool
	fed      bool
	checked  int
	first    uint64
	findings []Finding
	// open are the intents seen so far with no completion yet; overflow means
	// the set was too large to carry across pages and is incomplete.
	open     map[uint64]bool
	overflow bool
}

func (c *chainCheck) add(seq uint64, kind string) {
	if len(c.findings) < maxFindings {
		c.findings = append(c.findings, Finding{Seq: seq, Kind: kind})
	}
}

func (c *chainCheck) feed(e Event) {
	first := !c.fed
	c.fed = true
	c.checked++
	if c.first == 0 {
		c.first = e.Seq
	}
	if e.Tenant != c.tenant {
		c.add(e.Seq, FindingTenant)
	}
	if e.Validate() != nil {
		c.add(e.Seq, FindingInvalidEvent)
	}
	if e.Hash != e.ComputeHash() {
		c.add(e.Seq, FindingHash)
	}
	switch {
	case e.Seq <= c.expect.Seq:
		c.add(e.Seq, FindingReorder)
		return
	case e.Seq > c.expect.Seq+1:
		if first && !c.anchored {
			c.add(e.Seq, FindingUnanchored)
		} else {
			c.add(e.Seq, FindingGap)
		}
	case e.PrevHash != c.expect.Hash:
		if first && c.fromCP {
			c.add(e.Seq, FindingCheckpoint)
		} else {
			c.add(e.Seq, FindingPrev)
		}
	}
	if e.Time.Before(c.expect.Time) {
		c.add(e.Seq, FindingTime)
	}
	c.expect = HeadOf(e)
	switch {
	case e.Phase == PhaseIntent:
		if c.open == nil {
			c.open = map[uint64]bool{}
		}
		c.open[e.Seq] = true
	case e.Phase == PhaseCompletion && e.Ref != 0:
		delete(c.open, e.Ref)
	}
}

// finish compares where the chain ended with the stored head and checkpoint.
func (c *chainCheck) finish(head Head, cp *Checkpoint) {
	if cp != nil && cp.ThroughSeq > head.Seq {
		c.add(head.Seq, FindingCheckpoint)
	}
	if c.expect.Seq != head.Seq || c.expect.Hash != head.Hash {
		c.add(head.Seq, FindingHead)
	}
}

// anchorFor is where verification of a tenant starts: after the retention
// checkpoint, or at the genesis.
func anchorFor(cp *Checkpoint) (Head, bool) {
	if cp != nil {
		return Head{Seq: cp.ThroughSeq, Hash: cp.ThroughHash, Time: cp.ThroughTime}, true
	}
	return Head{Hash: Genesis}, false
}

// VerifyPage is the number of events one verification call examines.
const VerifyPage = 5000

var verifyPage = VerifyPage // a variable so tests can use small pages

// unresolvedAge is how old an intent must be before it counts as unresolved (a
// younger one may still be running); maxUnresolved bounds the listed seqs;
// maxCarriedOpen bounds the open intents a verification cursor carries from
// one page to the next.
const (
	unresolvedAge  = 5 * time.Minute
	maxUnresolved  = 50
	maxCarriedOpen = 64
)

// CheckpointInfo is what a verification reports about retention.
type CheckpointInfo struct {
	ThroughSeq uint64    `json:"through_seq"`
	PurgedAt   time.Time `json:"purged_at"`
}

// Unresolved lists intents older than five minutes with no completion
// anywhere in the verified retained range: operations in doubt (the write may
// or may not have happened, or its completion was lost). Complete is false
// when more than maxCarriedOpen intents were open at a page boundary of a
// multi-page verification and the state could not be carried: Count is then
// only a lower bound, never a claimed total.
type Unresolved struct {
	Count    int      `json:"count"`
	Seqs     []uint64 `json:"seqs"`
	Complete bool     `json:"complete"`
}

// VerifyResult is the outcome of one verification call.
type VerifyResult struct {
	Tenant     string          `json:"tenant"`
	OK         bool            `json:"ok"`
	Checked    int             `json:"checked"`
	FirstSeq   uint64          `json:"first_seq,omitempty"`
	LastSeq    uint64          `json:"last_seq,omitempty"`
	Checkpoint *CheckpointInfo `json:"checkpoint,omitempty"`
	// Complete is false while more events remain: NextCursor continues, and
	// the head check and UnresolvedIntents are only produced by the final call.
	Complete   bool      `json:"complete"`
	NextCursor string    `json:"next_cursor,omitempty"`
	Findings   []Finding `json:"findings"`
	// UnresolvedIntents is nil until the final call, which reports over the
	// WHOLE retained range (open intents are carried from page to page in the
	// MAC'd cursor): an intent whose completion sits on a page not yet examined
	// is never reported, and nothing is reported as zero that was not checked.
	UnresolvedIntents *Unresolved `json:"unresolved_intents,omitempty"`
}
