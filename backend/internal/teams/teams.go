// Package teams adds optional sign-in and shared workspaces (D68). The
// frontend proxy forwards the user's Google ID token as X-User-Token; a
// Verifier checks it and returns the user's email, and a Store maps the email
// to the one workspace the user has joined (by creating it, or by accepting
// an invitation). Cases created by a signed-in user then belong to that
// workspace, and its members see them. Anonymous visitors keep the
// visitor-ID ownership of D58.
package teams

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/idtoken"
)

// Errors returned by Verifier and Store.
var (
	ErrUnauthenticated = errors.New("teams: invalid sign-in token")
	ErrNotFound        = errors.New("teams: workspace not found")
	// ErrMember means the email already belongs to a workspace: a user is in
	// at most one, so "their workspace" is always well defined.
	ErrMember = errors.New("teams: already a member of a workspace")
	ErrFull   = fmt.Errorf("teams: a workspace has at most %d members", MaxMembers)
)

// MaxMembers caps a workspace's members.
const MaxMembers = 50

// MaxName caps a workspace name, in characters.
const MaxName = 100

// User is a signed-in user.
type User struct {
	Email string
}

// Verifier checks a sign-in token. Errors wrap ErrUnauthenticated.
type Verifier interface {
	Verify(ctx context.Context, token string) (User, error)
}

// googleIssuers are the issuers Google signs ID tokens as.
var googleIssuers = map[string]bool{"accounts.google.com": true, "https://accounts.google.com": true}

// Google verifies Google ID tokens (Sign in with Google) issued for one
// OAuth client ID: signature against Google's keys, expiry, audience and
// issuer, and that the email is verified.
type Google struct {
	audience string
	validate func(ctx context.Context, token, audience string) (*idtoken.Payload, error)
}

// NewGoogle returns a verifier for tokens issued to audience (the OAuth
// client ID). An empty audience would accept tokens issued to any client,
// so it is refused.
func NewGoogle(audience string) (*Google, error) {
	if audience == "" {
		return nil, errors.New("teams: the Google sign-in audience (OAuth client ID) is required")
	}
	return &Google{audience: audience, validate: idtoken.Validate}, nil
}

// Verify implements Verifier.
func (g *Google) Verify(ctx context.Context, token string) (User, error) {
	p, err := g.validate(ctx, token, g.audience)
	if err != nil {
		return User{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if p.Audience != g.audience { // idtoken checks this too; keep it explicit
		return User{}, fmt.Errorf("%w: audience %q", ErrUnauthenticated, p.Audience)
	}
	if !googleIssuers[p.Issuer] {
		return User{}, fmt.Errorf("%w: issuer %q", ErrUnauthenticated, p.Issuer)
	}
	if v, _ := p.Claims["email_verified"].(bool); !v {
		return User{}, fmt.Errorf("%w: email not verified", ErrUnauthenticated)
	}
	email, _ := p.Claims["email"].(string)
	email, ok := NormalEmail(email)
	if !ok {
		return User{}, fmt.Errorf("%w: no usable email", ErrUnauthenticated)
	}
	return User{Email: email}, nil
}

var emailPattern = regexp.MustCompile(`^[a-z0-9._%+-]{1,64}@[a-z0-9-]+(\.[a-z0-9-]+)*\.[a-z]{2,63}$`)

// NormalEmail lower-cases and checks an email. The pattern is stricter than
// RFC 5322 on purpose: the email is a Firestore document ID (no "/").
func NormalEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	return s, len(s) <= 254 && emailPattern.MatchString(s)
}

// Workspace is a team: its members see each other's cases. The creator is
// its admin and the only one who can invite. An invited email is not a
// member until that user signs in and accepts the invitation (Store.Join):
// nobody can be put in a workspace, and have their new cases shared with
// it, without their consent.
type Workspace struct {
	ID        string    `json:"id" firestore:"id"`
	Name      string    `json:"name" firestore:"name"`
	Members   []string  `json:"members" firestore:"members"`
	Invited   []string  `json:"invited" firestore:"invited"` // pending invitations
	CreatedBy string    `json:"created_by" firestore:"created_by"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
}

// IsAdmin reports whether email administers w.
func (w Workspace) IsAdmin(email string) bool { return email != "" && email == w.CreatedBy }

// HasMember reports whether email has joined w.
func (w Workspace) HasMember(email string) bool { return slices.Contains(w.Members, email) }

// HasInvite reports whether email has a pending invitation to w.
func (w Workspace) HasInvite(email string) bool { return slices.Contains(w.Invited, email) }

// accept moves email from the invitations to the members.
func (w *Workspace) accept(email string) {
	w.Invited = slices.DeleteFunc(slices.Clone(w.Invited), func(e string) bool { return e == email })
	w.Members = append(slices.Clone(w.Members), email)
}

// MaxInvites caps the pending invitations returned for one email.
const MaxInvites = 20

// Store keeps workspaces, their pending invitations, and which workspace
// each email has joined.
type Store interface {
	// Create saves a new workspace; ErrMember if any member already
	// belongs to a workspace.
	Create(ctx context.Context, w Workspace) error
	// Get returns a workspace; ErrNotFound if there is none.
	Get(ctx context.Context, id string) (Workspace, error)
	// Of returns the workspace email has joined; ErrNotFound if none. A
	// pending invitation is not a membership.
	Of(ctx context.Context, email string) (Workspace, error)
	// Invite records a pending invitation of email to workspace id; inviting
	// again is a no-op. ErrNotFound, ErrMember (already a member of this
	// workspace) or ErrFull (members plus invitations at MaxMembers).
	// Membership of another workspace is not checked: the invitation would
	// otherwise tell the admin which workspace someone is in.
	Invite(ctx context.Context, id, email string) (Workspace, error)
	// Invites returns up to MaxInvites workspaces that have invited email.
	Invites(ctx context.Context, email string) ([]Workspace, error)
	// Join accepts email's invitation to workspace id: ErrNotFound (no such
	// workspace or no invitation), ErrMember (email is already in a
	// workspace) or ErrFull.
	Join(ctx context.Context, id, email string) (Workspace, error)
}

// Memory is an in-process Store (local runs and tests).
type Memory struct {
	mu     sync.Mutex
	byID   map[string]Workspace
	member map[string]string // email -> workspace ID (joined only)
}

// NewMemory returns an empty store.
func NewMemory() *Memory { return &Memory{byID: map[string]Workspace{}, member: map[string]string{}} }

// Create implements Store.
func (m *Memory) Create(_ context.Context, w Workspace) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(w.Members)+len(w.Invited) > MaxMembers {
		return ErrFull
	}
	for _, e := range w.Members {
		if _, ok := m.member[e]; ok {
			return ErrMember
		}
	}
	w.Members, w.Invited = slices.Clone(w.Members), slices.Clone(w.Invited)
	m.byID[w.ID] = w
	for _, e := range w.Members {
		m.member[e] = w.ID
	}
	return nil
}

// Get implements Store.
func (m *Memory) Get(_ context.Context, id string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.get(id)
}

func (m *Memory) get(id string) (Workspace, error) {
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	w.Members, w.Invited = slices.Clone(w.Members), slices.Clone(w.Invited)
	return w, nil
}

// Of implements Store.
func (m *Memory) Of(_ context.Context, email string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id, ok := m.member[email]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	return m.get(id)
}

// Invite implements Store.
func (m *Memory) Invite(_ context.Context, id, email string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	switch {
	case !ok:
		return Workspace{}, ErrNotFound
	case w.HasMember(email):
		return Workspace{}, ErrMember
	case w.HasInvite(email):
		return m.get(id)
	case len(w.Members)+len(w.Invited) >= MaxMembers:
		return Workspace{}, ErrFull
	}
	w.Invited = append(slices.Clone(w.Invited), email)
	m.byID[id] = w
	return m.get(id)
}

// Invites implements Store, oldest workspace first.
func (m *Memory) Invites(_ context.Context, email string) ([]Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Workspace
	for id, w := range m.byID {
		if w.HasInvite(email) {
			c, _ := m.get(id)
			out = append(out, c)
		}
	}
	slices.SortFunc(out, func(a, b Workspace) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	if len(out) > MaxInvites {
		out = out[:MaxInvites]
	}
	return out, nil
}

// Join implements Store.
func (m *Memory) Join(_ context.Context, id, email string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok || !w.HasInvite(email) {
		return Workspace{}, ErrNotFound
	}
	if _, ok := m.member[email]; ok {
		return Workspace{}, ErrMember
	}
	if len(w.Members) >= MaxMembers {
		return Workspace{}, ErrFull
	}
	w.accept(email)
	m.byID[id] = w
	m.member[email] = id
	return m.get(id)
}

var _ Store = (*Memory)(nil)
