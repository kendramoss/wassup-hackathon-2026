package audit

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFilterByActorKind(t *testing.T) {
	c := &clock{t: t0}
	s := newTestService(t, NewMemory(), c.now)
	const ten = "ws:ws_0123456789ab"
	for i := 0; i < 6; i++ {
		c.advance(time.Minute)
		d := userDraft(ten, CaseExport)
		if i%3 == 0 {
			d.ActorKind, d.ActorID = ActorAPIKey, "0123456789abcdef"
		}
		mustRecord(t, s, d)
	}
	ctx := context.Background()
	keys, err := s.List(ctx, ten, ListQuery{Filter: Filter{ActorKind: ActorAPIKey}})
	if err != nil || len(keys.Events) != 2 {
		t.Fatalf("apikey events = %d %v", len(keys.Events), err)
	}
	users, _ := s.List(ctx, ten, ListQuery{Limit: 2, Desc: true, Filter: Filter{ActorKind: ActorUser}})
	if len(users.Events) != 2 || users.NextCursor == "" {
		t.Fatalf("user page = %d %q", len(users.Events), users.NextCursor)
	}
	if _, err := s.List(ctx, ten, ListQuery{Limit: 2, Desc: true, Cursor: users.NextCursor, Filter: Filter{ActorKind: ActorAPIKey}}); !errors.Is(err, ErrBadQuery) {
		t.Error("a cursor was replayed under another actor kind")
	}
	if !ValidActorKind(ActorSystem) || ValidActorKind("owner") {
		t.Error("ValidActorKind")
	}
}
