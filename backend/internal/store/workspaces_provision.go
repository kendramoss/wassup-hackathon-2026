package store

import (
	"context"

	"cloud.google.com/go/firestore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// Provision implements teams.Provisioner: the workspace document and the
// member's workspace_members document change in one transaction, and a
// member of another workspace is refused inside it.
func (s *FirestoreWorkspaces) Provision(ctx context.Context, id, email string, role teams.Role) (teams.Workspace, error) {
	if !caseIDPattern.MatchString(id) {
		return teams.Workspace{}, teams.ErrNotFound
	}
	var out teams.Workspace
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		w, err := decodeWorkspace(tx.Get(s.ws.Doc(id)))
		if err != nil {
			return err
		}
		snap, err := tx.Get(s.members.Doc(email))
		switch {
		case err == nil:
			var m memberDoc
			if err := snap.DataTo(&m); err != nil {
				return err
			}
			if m.Workspace != id {
				return teams.ErrMember
			}
		case status.Code(err) != codes.NotFound:
			return err
		}
		next, err := w.WithProvisioned(email, role)
		if err != nil {
			return err
		}
		out = next
		if err := tx.Set(s.ws.Doc(id), next); err != nil {
			return err
		}
		return tx.Set(s.members.Doc(email), memberDoc{Workspace: id})
	})
	if err != nil {
		return teams.Workspace{}, workspaceErr("provision member in", err)
	}
	return out, nil
}

// Deprovision implements teams.Provisioner.
func (s *FirestoreWorkspaces) Deprovision(ctx context.Context, id, email string) (teams.Workspace, error) {
	if !caseIDPattern.MatchString(id) {
		return teams.Workspace{}, teams.ErrNotFound
	}
	var out teams.Workspace
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		w, err := decodeWorkspace(tx.Get(s.ws.Doc(id)))
		if err != nil {
			return err
		}
		next, err := w.WithDeprovisioned(email)
		if err != nil {
			return err
		}
		out = next
		if err := tx.Set(s.ws.Doc(id), next); err != nil {
			return err
		}
		return tx.Delete(s.members.Doc(email))
	})
	if err != nil {
		return teams.Workspace{}, workspaceErr("deprovision member from", err)
	}
	return out, nil
}

var _ teams.Provisioner = (*FirestoreWorkspaces)(nil)
