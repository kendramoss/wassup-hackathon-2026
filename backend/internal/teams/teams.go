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
	// ErrForbidden means the acting member lacks the admin role the change needs.
	ErrForbidden = errors.New("teams: the workspace admin role is required")
	// ErrNotMember means the target email has not joined the workspace (an
	// invitation alone has no role and cannot be removed here).
	ErrNotMember = errors.New("teams: not a member of this workspace")
	// ErrLastAdmin means the change would leave the workspace without an admin.
	ErrLastAdmin = errors.New("teams: a workspace must keep at least one admin")
	// ErrInvalidRole means the role is not viewer, editor or admin.
	ErrInvalidRole = errors.New("teams: role must be viewer, editor or admin")
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

// Role is a member's authority inside one workspace (#283).
type Role string

// The roles, least to most authority.
const (
	RoleViewer Role = "viewer" // reads shared cases; export preview only
	RoleEditor Role = "editor" // viewer, plus runs and edits workspace cases
	RoleAdmin  Role = "admin"  // editor, plus invites, role changes, removals
)

// ParseRole checks a role name (exact, lower case).
func ParseRole(s string) (Role, bool) {
	switch r := Role(s); r {
	case RoleViewer, RoleEditor, RoleAdmin:
		return r, true
	}
	return "", false
}

// Workspace is a team: its members see each other's cases. Each member has a
// role (viewer, editor or admin); the admins invite, change roles and remove
// members. An invited email is not a member until that user signs in and
// accepts the invitation (Store.Join): nobody can be put in a workspace, and
// have their new cases shared with it, without their consent.
//
// Roles are stored explicitly (Roles, email -> role name). A document written
// before roles existed has none: its creator is then admin and every other
// member an editor (RoleOf derives it on read). A stored role always wins
// over that derivation, so a demoted creator is not admin again, and Join and
// Create write the role explicitly so a creator who left cannot return as
// admin by the creator rule.
type Workspace struct {
	ID        string    `json:"id" firestore:"id"`
	Name      string    `json:"name" firestore:"name"`
	Members   []string  `json:"members" firestore:"members"`
	Invited   []string  `json:"invited" firestore:"invited"` // pending invitations
	CreatedBy string    `json:"created_by" firestore:"created_by"`
	CreatedAt time.Time `json:"created_at" firestore:"created_at"`
	// Roles maps a joined member's email to a role name. Missing: derived (see
	// above). Invited emails never appear.
	Roles map[string]string `json:"roles,omitempty" firestore:"roles,omitempty"`
	// InvitedBy maps a pending invitee's email to the admin who first sent the
	// invitation (the trusted actor, #712). It is metadata beside Invited, which
	// stays the queried list. An invitation stored before this field existed
	// has no entry: its sender is unknown, never guessed from CreatedBy. An
	// entry goes when the invitation is used. Never sent to clients whole.
	InvitedBy map[string]string `json:"invited_by,omitempty" firestore:"invited_by,omitempty"`
}

// RoleOf returns email's role in w; ok is false unless email has joined w. An
// unrecognised stored value fails closed to viewer.
func (w Workspace) RoleOf(email string) (Role, bool) {
	if email == "" || !w.HasMember(email) {
		return "", false
	}
	if s, ok := w.Roles[email]; ok {
		if r, valid := ParseRole(s); valid {
			return r, true
		}
		return RoleViewer, true
	}
	if email == w.CreatedBy {
		return RoleAdmin, true
	}
	return RoleEditor, true
}

// IsAdmin reports whether email is a member of w with the admin role.
func (w Workspace) IsAdmin(email string) bool {
	r, ok := w.RoleOf(email)
	return ok && r == RoleAdmin
}

func (w Workspace) adminCount() int {
	n := 0
	for _, e := range w.Members {
		if w.IsAdmin(e) {
			n++
		}
	}
	return n
}

func (w Workspace) cloneRolesOrNil() map[string]string {
	if w.Roles == nil {
		return nil
	}
	return w.cloneRoles()
}

func cloneStrings(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (w Workspace) cloneRoles() map[string]string {
	out := make(map[string]string, len(w.Roles)+1)
	for k, v := range w.Roles {
		out[k] = v
	}
	return out
}

// InviterOf returns the admin who sent email's pending invitation; ok is
// false when email has no invitation or the invitation predates recording.
func (w Workspace) InviterOf(email string) (string, bool) {
	if !w.HasInvite(email) {
		return "", false
	}
	by, ok := w.InvitedBy[email]
	return by, ok && by != ""
}

// WithInvite returns w with a pending invitation of email made by actor: the
// whole invite rule (admin only, not already a member, MaxMembers over members
// plus invitations), run by a Store inside its atomic read-modify-write. An
// existing invitation is a no-op and keeps its first sender. w is not changed.
func (w Workspace) WithInvite(actor, email string) (Workspace, error) {
	switch {
	case !w.IsAdmin(actor):
		return Workspace{}, ErrForbidden
	case w.HasMember(email):
		return Workspace{}, ErrMember
	case w.HasInvite(email):
		return w, nil
	case len(w.Members)+len(w.Invited) >= MaxMembers:
		return Workspace{}, ErrFull
	}
	w.Members, w.Invited = slices.Clone(w.Members), append(slices.Clone(w.Invited), email)
	w.Roles = w.cloneRolesOrNil()
	w.InvitedBy = cloneStrings(w.InvitedBy)
	if w.InvitedBy == nil {
		w.InvitedBy = map[string]string{}
	}
	w.InvitedBy[email] = actor
	return w, nil
}

// WithRole returns w with target's role set by actor. It is the whole rule
// (admin only, target joined, valid role, never the last admin); a Store runs
// it inside its atomic read-modify-write so the check and the write cannot be
// separated.
func (w Workspace) WithRole(actor, target string, role Role) (Workspace, error) {
	if _, ok := ParseRole(string(role)); !ok {
		return Workspace{}, ErrInvalidRole
	}
	if !w.IsAdmin(actor) {
		return Workspace{}, ErrForbidden
	}
	cur, ok := w.RoleOf(target)
	if !ok {
		return Workspace{}, ErrNotMember
	}
	if cur == RoleAdmin && role != RoleAdmin && w.adminCount() <= 1 {
		return Workspace{}, ErrLastAdmin
	}
	w.Members, w.Invited = slices.Clone(w.Members), slices.Clone(w.Invited)
	w.Roles = w.cloneRoles()
	w.Roles[target] = string(role)
	return w, nil
}

// Without returns w with target removed by actor: an admin may remove any
// member, a member may remove themselves, and the last admin may not go.
func (w Workspace) Without(actor, target string) (Workspace, error) {
	if _, ok := w.RoleOf(actor); !ok {
		return Workspace{}, ErrForbidden
	}
	if actor != target && !w.IsAdmin(actor) {
		return Workspace{}, ErrForbidden
	}
	cur, ok := w.RoleOf(target)
	if !ok {
		return Workspace{}, ErrNotMember
	}
	if cur == RoleAdmin && w.adminCount() <= 1 {
		return Workspace{}, ErrLastAdmin
	}
	w.Members = slices.DeleteFunc(slices.Clone(w.Members), func(e string) bool { return e == target })
	w.Invited = slices.Clone(w.Invited)
	w.Roles = w.cloneRoles()
	delete(w.Roles, target)
	return w, nil
}

// HasMember reports whether email has joined w.
func (w Workspace) HasMember(email string) bool { return slices.Contains(w.Members, email) }

// HasInvite reports whether email has a pending invitation to w.
func (w Workspace) HasInvite(email string) bool { return slices.Contains(w.Invited, email) }

// Accept returns w with email moved from the invitations to the members, and
// the invitation's sender dropped. The new member is an editor, stored
// explicitly. w is not changed.
func (w Workspace) Accept(email string) Workspace {
	w.Invited = slices.DeleteFunc(slices.Clone(w.Invited), func(e string) bool { return e == email })
	w.Members = append(slices.Clone(w.Members), email)
	w.Roles = w.cloneRoles()
	w.Roles[email] = string(RoleEditor)
	if _, ok := w.InvitedBy[email]; ok {
		w.InvitedBy = cloneStrings(w.InvitedBy)
		delete(w.InvitedBy, email)
		if len(w.InvitedBy) == 0 {
			w.InvitedBy = nil
		}
	}
	return w
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
	// Invite records a pending invitation of email to workspace id, made by
	// actor; inviting again is a no-op. ErrNotFound, ErrForbidden (actor is
	// not an admin of the stored workspace, checked in the same atomic step as
	// the write), ErrMember (already a member of this workspace) or ErrFull
	// (members plus invitations at MaxMembers). Membership of another
	// workspace is not checked: the invitation would otherwise tell the admin
	// which workspace someone is in.
	Invite(ctx context.Context, id, actor, email string) (Workspace, error)
	// SetRole sets a joined member's role (Workspace.WithRole). The admin check
	// and the last-admin check read the stored workspace in the same atomic
	// step as the write. ErrNotFound, ErrForbidden, ErrNotMember,
	// ErrInvalidRole, ErrLastAdmin.
	SetRole(ctx context.Context, id, actor, target string, role Role) (Workspace, error)
	// Remove takes target out of the workspace (Workspace.Without; atomic, and
	// target may join again only by a new invitation). ErrNotFound,
	// ErrForbidden, ErrNotMember, ErrLastAdmin.
	Remove(ctx context.Context, id, actor, target string) (Workspace, error)
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
	w.Members, w.Invited, w.Roles, w.InvitedBy = slices.Clone(w.Members), slices.Clone(w.Invited), w.cloneRolesOrNil(), cloneStrings(w.InvitedBy)
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
	w.Members, w.Invited, w.Roles, w.InvitedBy = slices.Clone(w.Members), slices.Clone(w.Invited), w.cloneRolesOrNil(), cloneStrings(w.InvitedBy)
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
func (m *Memory) Invite(_ context.Context, id, actor, email string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	out, err := w.WithInvite(actor, email)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = out
	return m.get(id)
}

// SetRole implements Store.
func (m *Memory) SetRole(_ context.Context, id, actor, target string, role Role) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	out, err := w.WithRole(actor, target, role)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = out
	return m.get(id)
}

// Remove implements Store.
func (m *Memory) Remove(_ context.Context, id, actor, target string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	out, err := w.Without(actor, target)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = out
	delete(m.member, target)
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
	m.byID[id] = w.Accept(email)
	m.member[email] = id
	return m.get(id)
}

var _ Store = (*Memory)(nil)
