package store

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Workspace collections (D68): one document per workspace, and one per
// member email naming the member's workspace, so "which workspace is this
// user in" is a single read and a user cannot join two.
const (
	WorkspaceCollection = "workspaces"
	MemberCollection    = "workspace_members"
)

// Workspaces is the workspace store on the same client.
func (f *Firestore) Workspaces() *FirestoreWorkspaces {
	return &FirestoreWorkspaces{client: f.client, ws: f.client.Collection(WorkspaceCollection), members: f.client.Collection(MemberCollection)}
}

// FirestoreWorkspaces implements teams.Store. Writes run in transactions so
// a member document and its workspace never disagree.
type FirestoreWorkspaces struct {
	client      *firestore.Client
	ws, members *firestore.CollectionRef
}

type memberDoc struct {
	Workspace string `firestore:"workspace_id"`
}

// Create implements teams.Store.
func (s *FirestoreWorkspaces) Create(ctx context.Context, w teams.Workspace) error {
	if len(w.Members) > teams.MaxMembers {
		return teams.ErrFull
	}
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		for _, e := range w.Members {
			if _, err := tx.Get(s.members.Doc(e)); err == nil {
				return teams.ErrMember
			} else if status.Code(err) != codes.NotFound {
				return err
			}
		}
		if err := tx.Create(s.ws.Doc(w.ID), w); err != nil {
			return err
		}
		for _, e := range w.Members {
			if err := tx.Create(s.members.Doc(e), memberDoc{Workspace: w.ID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, teams.ErrMember) && !errors.Is(err, teams.ErrFull) {
		return fmt.Errorf("store: create workspace: %w", err)
	}
	return err
}

// Get implements teams.Store.
func (s *FirestoreWorkspaces) Get(ctx context.Context, id string) (teams.Workspace, error) {
	if !caseIDPattern.MatchString(id) {
		return teams.Workspace{}, teams.ErrNotFound
	}
	snap, err := s.ws.Doc(id).Get(ctx)
	return decodeWorkspace(snap, err)
}

// Of implements teams.Store.
func (s *FirestoreWorkspaces) Of(ctx context.Context, email string) (teams.Workspace, error) {
	snap, err := s.members.Doc(email).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return teams.Workspace{}, teams.ErrNotFound
	}
	if err != nil {
		return teams.Workspace{}, fmt.Errorf("store: read member: %w", err)
	}
	var m memberDoc
	if err := snap.DataTo(&m); err != nil {
		return teams.Workspace{}, fmt.Errorf("store: decode member: %w", err)
	}
	return s.Get(ctx, m.Workspace)
}

// AddMember implements teams.Store.
func (s *FirestoreWorkspaces) AddMember(ctx context.Context, id, email string) (teams.Workspace, error) {
	if !caseIDPattern.MatchString(id) {
		return teams.Workspace{}, teams.ErrNotFound
	}
	var out teams.Workspace
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		w, err := decodeWorkspace(tx.Get(s.ws.Doc(id)))
		if err != nil {
			return err
		}
		if _, err := tx.Get(s.members.Doc(email)); err == nil {
			return teams.ErrMember
		} else if status.Code(err) != codes.NotFound {
			return err
		}
		if len(w.Members) >= teams.MaxMembers {
			return teams.ErrFull
		}
		w.Members = append(w.Members, email)
		if err := tx.Set(s.ws.Doc(id), w); err != nil {
			return err
		}
		out = w
		return tx.Create(s.members.Doc(email), memberDoc{Workspace: id})
	})
	switch {
	case err == nil:
		return out, nil
	case errors.Is(err, teams.ErrNotFound), errors.Is(err, teams.ErrMember), errors.Is(err, teams.ErrFull):
		return teams.Workspace{}, err
	}
	return teams.Workspace{}, fmt.Errorf("store: add member: %w", err)
}

func decodeWorkspace(snap *firestore.DocumentSnapshot, err error) (teams.Workspace, error) {
	if status.Code(err) == codes.NotFound {
		return teams.Workspace{}, teams.ErrNotFound
	}
	if err != nil {
		return teams.Workspace{}, fmt.Errorf("store: read workspace: %w", err)
	}
	var w teams.Workspace
	if err := snap.DataTo(&w); err != nil {
		return teams.Workspace{}, fmt.Errorf("store: decode workspace %s: %w", snap.Ref.ID, err)
	}
	return w, nil
}

var _ teams.Store = (*FirestoreWorkspaces)(nil)
