package teams

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"
)

// Renamer changes a workspace's name (#870). Both built-in stores are one.
type Renamer interface {
	// Rename sets the name of workspace id; the admin check reads the stored
	// workspace in the same atomic step as the write. ErrNotFound,
	// ErrForbidden, ErrInvalidName.
	Rename(ctx context.Context, id, actor, name string) (Workspace, error)
}

// ErrInvalidName means a name is empty or longer than MaxName.
var ErrInvalidName = errInvalidName{}

type errInvalidName struct{}

func (errInvalidName) Error() string { return "teams: name must be 1 to 100 characters" }

// CleanName collapses the whitespace of a name and checks its length.
func CleanName(s string) (string, bool) {
	s = strings.Join(strings.Fields(s), " ")
	n := utf8.RuneCountInString(s)
	return s, n >= 1 && n <= MaxName
}

// WithName returns w renamed by actor, who must be an admin. w is not changed.
func (w Workspace) WithName(actor, name string) (Workspace, error) {
	name, ok := CleanName(name)
	if !ok {
		return Workspace{}, ErrInvalidName
	}
	if !w.IsAdmin(actor) {
		return Workspace{}, ErrForbidden
	}
	w.Members, w.Invited = slices.Clone(w.Members), slices.Clone(w.Invited)
	w.Roles = w.cloneRolesOrNil()
	w.Name = name
	return w, nil
}

// Rename implements Renamer.
func (m *Memory) Rename(_ context.Context, id, actor, name string) (Workspace, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.byID[id]
	if !ok {
		return Workspace{}, ErrNotFound
	}
	next, err := w.WithName(actor, name)
	if err != nil {
		return Workspace{}, err
	}
	m.byID[id] = next
	return m.get(id)
}

var _ Renamer = (*Memory)(nil)
