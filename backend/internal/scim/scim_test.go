package scim

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestSecretsAreHashedAndWellFormed(t *testing.T) {
	s, h := NewSecret()
	if !WellFormed(s) || Hash(s) != h || strings.Contains(h, s[len(TokenPrefix):]) {
		t.Fatalf("secret %q hash %q", s, h)
	}
	s2, h2 := NewSecret()
	if s == s2 || h == h2 {
		t.Fatal("secrets repeat")
	}
	for _, bad := range []string{"", "scim_", "scim_short", "op_" + s[5:], s + "x", strings.Replace(s, s[10:11], "!", 1)} {
		if WellFormed(bad) {
			t.Errorf("WellFormed(%q)", bad)
		}
	}
}

func TestMemoryStoreReplacesPerWorkspace(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	_, h1 := NewSecret()
	_, h2 := NewSecret()
	_ = m.Put(ctx, Token{Workspace: "ws1", Hash: h1})
	_ = m.Put(ctx, Token{Workspace: "ws1", Hash: h2})
	if _, err := m.ByHash(ctx, h1); err != ErrNotFound {
		t.Fatal("a replaced token still works")
	}
	if tk, err := m.ByHash(ctx, h2); err != nil || tk.Workspace != "ws1" {
		t.Fatal(err)
	}
	_ = m.Delete(ctx, "ws1")
	if _, err := m.ByHash(ctx, h2); err != ErrNotFound {
		t.Fatal("a revoked token still works")
	}
}

func TestManagesOnlyItsDomains(t *testing.T) {
	tk := Token{Domains: []string{"acme.com"}}
	for email, want := range map[string]bool{"a@acme.com": true, "a@evil.acme.com": false, "a@acme.com.evil": false, "acme.com": false, "a@other.com": false} {
		if tk.Manages(email) != want {
			t.Errorf("Manages(%q) = %v", email, !want)
		}
	}
}

func TestUserIDsDifferPerWorkspace(t *testing.T) {
	a, b := UserID("ws1", "a@acme.com"), UserID("ws2", "a@acme.com")
	if a == b || strings.Contains(a, "acme") || len(a) != 25 {
		t.Fatalf("%s %s", a, b)
	}
}

func TestParseFilter(t *testing.T) {
	for f, want := range map[string]string{`userName eq "A@Acme.com"`: "a@acme.com", `USERNAME EQ "b@acme.com"`: "b@acme.com", `emails.value eq "c@acme.com"`: "c@acme.com"} {
		got, all, err := ParseFilter(f)
		if err != nil || all || got != want {
			t.Errorf("%s = %q %v %v", f, got, all, err)
		}
	}
	if _, all, err := ParseFilter(""); err != nil || !all {
		t.Error("empty filter")
	}
	for _, f := range []string{`userName co "a"`, `userName eq "a" or userName eq "b"`, `displayName eq "x"`, `userName eq "a\"b"`} {
		if _, _, err := ParseFilter(f); err != ErrFilter {
			t.Errorf("%s accepted", f)
		}
	}
}

func TestParseRoles(t *testing.T) {
	for in, want := range map[string]string{``: "", `null`: "", `[]`: "", `"Editor"`: "editor", `[{"value":"admin","primary":true}]`: "admin", `["viewer"]`: "viewer"} {
		got, err := ParseRoles(json.RawMessage(in))
		if err != nil || got != want {
			t.Errorf("%s = %q %v", in, got, err)
		}
	}
	for _, in := range []string{`"owner"`, `[{"value":"root"}]`, `[{"value":"viewer"},{"value":"admin"}]`, `42`} {
		if _, err := ParseRoles(json.RawMessage(in)); err == nil {
			t.Errorf("%s accepted", in)
		}
	}
}

func TestParsePatch(t *testing.T) {
	cases := map[string]Change{
		`{"Operations":[{"op":"replace","path":"active","value":false}]}`:                             {Active: ptr(false)},
		`{"Operations":[{"op":"Replace","path":"active","value":"False"}]}`:                           {Active: ptr(false)},
		`{"Operations":[{"op":"replace","value":{"active":true}}]}`:                                   {Active: ptr(true)},
		`{"Operations":[{"op":"add","path":"roles","value":[{"value":"editor"}]}]}`:                   {Role: "editor"},
		`{"Operations":[{"op":"remove","path":"roles[value eq \"editor\"]"}]}`:                        {},
		`{"Operations":[{"op":"replace","value":{"active":true,"displayName":"x","roles":"admin"}}]}`: {Active: ptr(true), Role: "admin"},
	}
	for body, want := range cases {
		var req PatchRequest
		_ = json.Unmarshal([]byte(body), &req)
		got, err := ParsePatch(req)
		if err != nil || got.Role != want.Role || (got.Active == nil) != (want.Active == nil) || got.Active != nil && *got.Active != *want.Active {
			t.Errorf("%s = %+v %v", body, got, err)
		}
	}
	for _, body := range []string{`{"Operations":[]}`, `{"Operations":[{"op":"replace","path":"userName","value":"x@y.com"}]}`,
		`{"Operations":[{"op":"replace","path":"active","value":"maybe"}]}`, `{"Operations":[{"op":"remove","path":"active"}]}`,
		`{"Operations":[{"op":"move","path":"active","value":true}]}`} {
		var req PatchRequest
		_ = json.Unmarshal([]byte(body), &req)
		if _, err := ParsePatch(req); err == nil {
			t.Errorf("%s accepted", body)
		}
	}
}

func TestInputEmail(t *testing.T) {
	if e := (UserInput{UserName: " A@Acme.COM "}).Email(); e != "a@acme.com" {
		t.Error(e)
	}
	if e := (UserInput{UserName: "abc123", Emails: []Value{{Value: "b@acme.com", Primary: true}}}).Email(); e != "b@acme.com" {
		t.Error(e)
	}
	if e := (UserInput{UserName: "not an email"}).Email(); e != "" {
		t.Error(e)
	}
}

func ptr[T any](v T) *T { return &v }
