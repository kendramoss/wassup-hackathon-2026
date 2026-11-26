package teams

import (
	"context"
	"slices"
)

// Provisioner adds and removes members on behalf of the workspace's identity
// provider (SSO just-in-time sign-in, SCIM, #868 #869), with no acting member.
// The caller has already authenticated the provider for this workspace: a
// verified SSO connection or the workspace's SCIM token. Nothing here reads a
// request.
type Provisioner interface {
	// Provision makes email a member of workspace id with role. A member of
	// this workspace keeps their role (Provision is idempotent); a pending
	// invitation is used up. ErrNotFound (no workspace), ErrMember (email is in
	// another workspace), ErrFull, ErrInvalidRole.
	Provision(ctx context.Context, id, email string, role Role) (Workspace, error)
	// Deprovision removes email from workspace id. ErrNotFound, ErrNotMember,
	// ErrLastAdmin (the identity provider cannot leave a workspace with no
	// admin).
	Deprovision(ctx context.Context, id, email string) (Workspace, error)
}

// WithProvisioned returns w with email a member with role, or w unchanged if
// email has already joined. The caller checks that email is in no other
// workspace. w is not changed.
func (w Workspace) WithProvisioned(email string, role Role) (Workspace, error) {
	if _, ok := ParseRole(string(role)); !ok {
		return Workspace{}, ErrInvalidRole
	}
	if w.HasMember(email) {
		return w, nil
	}
	if len(w.Members) >= MaxMembers {
		return Workspace{}, ErrFull
	}
	if w.HasInvite(email) {
		w = w.Accept(email)
	} else {
		w.Members = append(slices.Clone(w.Members), email)
		w.Invited = slices.Clone(w.Invited)
		w.Roles = w.cloneRoles()
	}
	w.Roles[email] = string(role)
	return w, nil
}

// WithDeprovisioned returns w without email. The last admin cannot be removed.
func (w Workspace) WithDeprovisioned(email string) (Workspace, error) {
	cur, ok := w.RoleOf(email)
	if !ok {
		return Workspace{}, ErrNotMember
	}
	if cur == RoleAdmin && w.adminCount() <= 1 {
		return Workspace{}, ErrLastAdmin
	}
	w.Members = slices.DeleteFunc(slices.Clone(w.Members), func(e string) bool { return e == email })
	w.Invited = slices.Clone(w.Invited)
	w.Roles = w.cloneRoles()
	delete(w.Roles, email)
	return w, nil
}

// Provision implements Provisioner.
func (m *Memory) Provision(_ context.Context, id, email string, role Role) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	if other, ok := m.member[email]; ok && other != id {
		return Workspace{}, ErrMember
	}
	next, err := w.WithProvisioned(email, role)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = next
	m.member[email] = id
	return m.get(id)
}

// Deprovision implements Provisioner.
func (m *Memory) Deprovision(_ context.Context, id, email string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	next, err := w.WithDeprovisioned(email)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = next
	delete(m.member, email)
	return m.get(id)
}

var _ Provisioner = (*Memory)(nil)
