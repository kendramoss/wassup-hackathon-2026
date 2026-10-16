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
	got, err := m.Invite(ctx, "ws_a", "amy@example.com")
	if err != nil || len(got.Members) != 1 || !got.HasInvite("amy@example.com") || got.HasMember("amy@example.com") {
		t.Fatalf("Invite = %+v %v", got, err)
	}
	if _, err := m.Of(ctx, "amy@example.com"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Of(amy) before joining = %v, want ErrNotFound", err)
	}
	if again, err := m.Invite(ctx, "ws_a", "amy@example.com"); err != nil || len(again.Invited) != 1 {
		t.Errorf("inviting amy twice = %+v %v, want a no-op", again, err)
	}
	if inv, err := m.Invites(ctx, "amy@example.com"); err != nil || len(inv) != 1 || inv[0].ID != "ws_a" {
		t.Errorf("Invites(amy) = %+v %v", inv, err)
	}
	if _, err := m.Invite(ctx, "ws_a", "alice@example.com"); !errors.Is(err, ErrMember) {
		t.Errorf("inviting a member = %v, want ErrMember", err)
	}
	if _, err := m.Invite(ctx, "ws_x", "zed@example.com"); !errors.Is(err, ErrNotFound) {
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
	if _, err := m.Invite(ctx, "ws_b", "amy@example.com"); err != nil {
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
		if _, err := m.Invite(ctx, "ws_a", fmt.Sprintf("u%d@example.com", i)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.Invite(ctx, "ws_a", "late@example.com"); !errors.Is(err, ErrFull) {
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
		if _, err := m.Invite(ctx, id, "victim@example.com"); err != nil {
			t.Fatal(err)
		}
	}
	if inv, err := m.Invites(ctx, "victim@example.com"); err != nil || len(inv) != MaxInvites || inv[0].ID != "ws_00" {
		t.Errorf("Invites = %d %v", len(inv), err)
	}
}
