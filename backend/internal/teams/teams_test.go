package teams

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"google.golang.org/api/idtoken"
)

const aud = "1234-client.apps.googleusercontent.com"

func payload(iss, aud string, claims map[string]any) *idtoken.Payload {
	return &idtoken.Payload{Issuer: iss, Audience: aud, Claims: claims}
}

func TestGoogleVerify(t *testing.T) {
	ok := map[string]any{"email": "Alice@Example.COM", "email_verified": true}
	tests := []struct {
		name string
		p    *idtoken.Payload
		err  error // from the signature check
		want string
	}{
		{"valid, email lower-cased", payload("https://accounts.google.com", aud, ok), nil, "alice@example.com"},
		{"short issuer", payload("accounts.google.com", aud, ok), nil, "alice@example.com"},
		{"bad signature or expired", nil, errors.New("idtoken: token expired"), ""},
		{"other issuer", payload("https://evil.example", aud, ok), nil, ""},
		{"other audience", payload("https://accounts.google.com", "other-client", ok), nil, ""},
		{"unverified email", payload("https://accounts.google.com", aud, map[string]any{"email": "a@example.com", "email_verified": false}), nil, ""},
		{"verified as a string", payload("https://accounts.google.com", aud, map[string]any{"email": "a@example.com", "email_verified": "true"}), nil, ""},
		{"no email", payload("https://accounts.google.com", aud, map[string]any{"email_verified": true}), nil, ""},
		{"email with a slash", payload("https://accounts.google.com", aud, map[string]any{"email": "a/b@example.com", "email_verified": true}), nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotAud string
			g := &Google{audience: aud, validate: func(_ context.Context, _, a string) (*idtoken.Payload, error) {
				gotAud = a
				return tt.p, tt.err
			}}
			u, err := g.Verify(context.Background(), "token")
			if gotAud != aud {
				t.Errorf("validated against audience %q", gotAud)
			}
			if tt.want == "" {
				if !errors.Is(err, ErrUnauthenticated) {
					t.Errorf("err = %v, want ErrUnauthenticated", err)
				}
				return
			}
			if err != nil || u.Email != tt.want {
				t.Errorf("Verify = %+v %v, want %s", u, err, tt.want)
			}
		})
	}
	if _, err := NewGoogle(""); err == nil {
		t.Error("an empty audience was accepted (it would skip the audience check)")
	}
	// The real validator refuses a malformed token before fetching keys.
	g, err := NewGoogle(aud)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Verify(context.Background(), "not-a-jwt"); !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("malformed token = %v", err)
	}
}

func TestNormalEmail(t *testing.T) {
	for in, want := range map[string]bool{"a.b+c@ex-ample.co.uk": true, " Bob@Example.com ": true, "a@b": false, "@b.com": false, "a b@c.com": false, "": false} {
		if _, ok := NormalEmail(in); ok != want {
			t.Errorf("NormalEmail(%q) ok = %v, want %v", in, ok, want)
		}
	}
	if e, _ := NormalEmail(" Bob@Example.com "); e != "bob@example.com" {
		t.Errorf("normalised = %q", e)
	}
}

func TestMemory(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if _, err := m.Of(ctx, "alice@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Of before create = %v", err)
	}
	w := Workspace{ID: "ws_a", Name: "Acme", Members: []string{"alice@example.com"}, CreatedBy: "alice@example.com"}
	if err := m.Create(ctx, w); err != nil {
		t.Fatal(err)
	}
	if err := m.Create(ctx, Workspace{ID: "ws_b", Members: []string{"alice@example.com"}}); !errors.Is(err, ErrMember) {
		t.Errorf("second workspace for alice = %v, want ErrMember", err)
	}
	if _, err := m.Get(ctx, "ws_b"); !errors.Is(err, ErrNotFound) {
		t.Errorf("refused workspace was saved: %v", err)
	}

	// An invitation is not a membership.
	got, err := m.Invite(ctx, "ws_a", "alice@example.com", "amy@example.com")
	if err != nil || len(got.Members) != 1 || !got.HasInvite("amy@example.com") || got.HasMember("amy@example.com") {
		t.Fatalf("Invite = %+v %v", got, err)
	}
	if _, err := m.Of(ctx, "amy@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Of(amy) before joining = %v, want ErrNotFound", err)
	}
	if again, err := m.Invite(ctx, "ws_a", "alice@example.com", "amy@example.com"); err != nil || len(again.Invited) != 1 {
		t.Errorf("inviting amy twice = %+v %v, want a no-op", again, err)
	}
	if inv, err := m.Invites(ctx, "amy@example.com"); err != nil || len(inv) != 1 || inv[0].ID != "ws_a" {
		t.Errorf("Invites(amy) = %+v %v", inv, err)
	}
	if _, err := m.Invite(ctx, "ws_a", "alice@example.com", "alice@example.com"); !errors.Is(err, ErrMember) {
		t.Errorf("inviting a member = %v, want ErrMember", err)
	}
	if _, err := m.Invite(ctx, "ws_x", "alice@example.com", "zed@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("invite to an unknown workspace = %v", err)
	}
	if _, err := m.Join(ctx, "ws_a", "zed@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("joining uninvited = %v, want ErrNotFound", err)
	}
	if _, err := m.Join(ctx, "ws_x", "amy@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("joining an unknown workspace = %v, want ErrNotFound", err)
	}

	got, err = m.Join(ctx, "ws_a", "amy@example.com")
	if err != nil || !got.HasMember("amy@example.com") || got.HasInvite("amy@example.com") {
		t.Fatalf("Join = %+v %v", got, err)
	}
	got.Members[0] = "changed" // callers get a copy
	if of, err := m.Of(ctx, "amy@example.com"); err != nil || of.ID != "ws_a" || of.Members[0] != "alice@example.com" {
		t.Errorf("Of(amy) = %+v %v", of, err)
	}
	if inv, _ := m.Invites(ctx, "amy@example.com"); len(inv) != 0 {
		t.Errorf("invitation kept after joining: %+v", inv)
	}
	if _, err := m.Join(ctx, "ws_a", "amy@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("joining twice = %v, want ErrNotFound (invitation used)", err)
	}

	// Someone already in a workspace can be invited (an invite does not
	// reveal their membership) but cannot join a second one.
	if err := m.Create(ctx, Workspace{ID: "ws_b", Name: "Bob Co", Members: []string{"bob@example.com"}, CreatedBy: "bob@example.com"}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Invite(ctx, "ws_b", "bob@example.com", "amy@example.com"); err != nil {
		t.Errorf("inviting a member of another workspace = %v", err)
	}
	if _, err := m.Join(ctx, "ws_b", "amy@example.com"); !errors.Is(err, ErrMember) {
		t.Errorf("amy joins a second workspace = %v, want ErrMember", err)
	}
	if of, _ := m.Of(ctx, "amy@example.com"); of.ID != "ws_a" {
		t.Errorf("amy moved to %q", of.ID)
	}

	// Members plus invitations are capped.
	for i := 2; i < MaxMembers; i++ {
		if _, err := m.Invite(ctx, "ws_a", "alice@example.com", fmt.Sprintf("u%d@example.com", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Invite(ctx, "ws_a", "alice@example.com", "late@example.com"); !errors.Is(err, ErrFull) {
		t.Errorf("invitation %d = %v, want ErrFull", MaxMembers+1, err)
	}
	if !w.IsAdmin("alice@example.com") || w.IsAdmin("amy@example.com") || (Workspace{}).IsAdmin("") {
		t.Error("IsAdmin")
	}
}

func TestMemoryInvitesCapped(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	for i := 0; i < MaxInvites+3; i++ {
		id := fmt.Sprintf("ws_%02d", i)
		admin := fmt.Sprintf("admin%d@example.com", i)
		if err := m.Create(ctx, Workspace{ID: id, Members: []string{admin}, CreatedBy: admin}); err != nil {
			t.Fatal(err)
		}
		if _, err := m.Invite(ctx, id, admin, "victim@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if inv, err := m.Invites(ctx, "victim@example.com"); err != nil || len(inv) != MaxInvites || inv[0].ID != "ws_00" {
		t.Errorf("Invites = %d %v", len(inv), err)
	}
}

// Issue #712: the store records the admin who sent each invitation.
func TestInviterIsRecordedPerInvitation(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	const alice, bob, carl = "alice@example.com", "bob@example.com", "carl@example.com"
	if err := m.Create(ctx, Workspace{ID: "ws_a", Name: "Acme", Members: []string{alice, bob}, CreatedBy: alice,
		Roles: map[string]string{alice: "admin", bob: "admin"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Invite(ctx, "ws_a", alice, "x@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Invite(ctx, "ws_a", bob, "y@example.com"); err != nil {
		t.Fatal(err)
	}
	// Another admin inviting the same email again keeps the first sender.
	got, err := m.Invite(ctx, "ws_a", bob, "x@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if by, ok := got.InviterOf("x@example.com"); !ok || by != alice {
		t.Errorf("x invited by %q %v, want alice (first sender kept)", by, ok)
	}
	if by, ok := got.InviterOf("y@example.com"); !ok || by != bob {
		t.Errorf("y invited by %q %v, want bob", by, ok)
	}
	if _, ok := got.InviterOf("nobody@example.com"); ok {
		t.Error("an email with no invitation has an inviter")
	}

	// The creator leaving does not change who sent an invitation.
	if _, err := m.Remove(ctx, "ws_a", bob, alice); err != nil {
		t.Fatal(err)
	}
	if _, err := m.SetRole(ctx, "ws_a", bob, bob, RoleAdmin); err != nil {
		t.Fatal(err)
	}
	inv, err := m.Invites(ctx, "x@example.com")
	if err != nil || len(inv) != 1 {
		t.Fatalf("Invites = %v %v", inv, err)
	}
	if by, ok := inv[0].InviterOf("x@example.com"); !ok || by != alice {
		t.Errorf("after alice left: x invited by %q %v, want alice", by, ok)
	}

	// Callers get a copy of the metadata.
	inv[0].InvitedBy["x@example.com"] = carl
	again, _ := m.Get(ctx, "ws_a")
	if by, _ := again.InviterOf("x@example.com"); by != alice {
		t.Errorf("a caller's change reached the store: %q", by)
	}

	// Joining consumes the invitation and its metadata, so a later invitation
	// is attributed to its own sender.
	joined, err := m.Join(ctx, "ws_a", "x@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := joined.InvitedBy["x@example.com"]; ok {
		t.Error("inviter kept after the invitation was used")
	}
	if _, ok := joined.InviterOf("x@example.com"); ok {
		t.Error("InviterOf answers for a member")
	}
	if by, ok := joined.InviterOf("y@example.com"); !ok || by != bob {
		t.Errorf("another invitation's sender changed: %q %v", by, ok)
	}
	if _, err := m.Remove(ctx, "ws_a", bob, "x@example.com"); err != nil {
		t.Fatal(err)
	}
	got, err = m.Invite(ctx, "ws_a", bob, "x@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if by, _ := got.InviterOf("x@example.com"); by != bob {
		t.Errorf("re-invitation attributed to %q, want bob", by)
	}
}

// A workspace stored before inviters were recorded has pending invitations
// with no sender: InviterOf says so instead of guessing the creator.
func TestInviterOfLegacyInvitationIsUnknown(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.Create(ctx, Workspace{ID: "ws_old", Members: []string{"alice@example.com"}, Invited: []string{"zed@example.com"}, CreatedBy: "alice@example.com"}); err != nil {
		t.Fatal(err)
	}
	w, _ := m.Get(ctx, "ws_old")
	if by, ok := w.InviterOf("zed@example.com"); ok || by != "" {
		t.Errorf("legacy inviter = %q %v, want unknown", by, ok)
	}
	// Joining a legacy invitation works and creates no metadata.
	joined, err := m.Join(ctx, "ws_old", "zed@example.com")
	if err != nil || len(joined.InvitedBy) != 0 {
		t.Errorf("Join legacy = %+v %v", joined, err)
	}
}

// WithInvite is the whole invite rule the Firestore store runs in its
// transaction: it must not change its input, and keeps the D303 limits.
func TestWithInviteRule(t *testing.T) {
	const alice, bob = "alice@example.com", "bob@example.com"
	base := Workspace{ID: "ws", Members: []string{alice, bob}, CreatedBy: alice, Roles: map[string]string{alice: "admin", bob: "editor"}}
	if _, err := base.WithInvite(bob, "z@example.com"); !errors.Is(err, ErrForbidden) {
		t.Errorf("editor invites = %v, want ErrForbidden", err)
	}
	if _, err := base.WithInvite(alice, bob); !errors.Is(err, ErrMember) {
		t.Errorf("invite a member = %v, want ErrMember", err)
	}
	next, err := base.WithInvite(alice, "z@example.com")
	if err != nil || !next.HasInvite("z@example.com") {
		t.Fatalf("WithInvite = %+v %v", next, err)
	}
	if len(base.Invited) != 0 || len(base.InvitedBy) != 0 {
		t.Errorf("input changed: %+v", base)
	}
	if by, _ := next.InviterOf("z@example.com"); by != alice {
		t.Errorf("inviter = %q", by)
	}
	full := base
	for i := 0; len(full.Members)+len(full.Invited) < MaxMembers; i++ {
		full.Invited = append(full.Invited, fmt.Sprintf("u%d@example.com", i))
	}
	if _, err := full.WithInvite(alice, "late@example.com"); !errors.Is(err, ErrFull) {
		t.Errorf("past the cap = %v, want ErrFull", err)
	}
	// The same invitation again at the cap is still a no-op, as before.
	if same, err := full.WithInvite(alice, "u0@example.com"); err != nil || len(same.Invited) != len(full.Invited) {
		t.Errorf("repeat at the cap = %v %v", len(same.Invited), err)
	}
	// Accept clears the sender without touching the input's map.
	pending, _ := base.WithInvite(alice, "z@example.com")
	accepted := pending.Accept("z@example.com")
	if _, ok := accepted.InvitedBy["z@example.com"]; ok || !accepted.HasMember("z@example.com") {
		t.Errorf("Accept = %+v", accepted)
	}
	if _, ok := pending.InvitedBy["z@example.com"]; !ok {
		t.Error("Accept changed its receiver's map")
	}
}
