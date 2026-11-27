package placement

import (
	"context"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Wrap returns cs guarded by the placement: a case outside it is reported as
// not found, and a write outside it is refused with store.ErrNotFound too,
// so the answer does not reveal that another tenant's or region's case
// exists. With an inactive placement it returns cs itself.
//
// Pinned demo cases are shared material, not customer data: a dedicated
// deployment still serves them. They are region-tagged like any other case.
func Wrap(cs store.Cases, c Config) store.Cases {
	if !c.Active() {
		return cs
	}
	return &cases{inner: cs, c: c}
}

type cases struct {
	inner store.Cases
	c     Config
}

// Allows reports whether the case belongs to the placement. A case with no
// region tag was written before residency was on: it is allowed (and gets
// the tag on its next write), since the owner moves data before turning a
// region on (docs/design/data-residency.md).
func (c Config) Allows(sc store.Case) bool {
	if c.Region != "" && sc.DataRegion != "" && sc.DataRegion != c.Region {
		return false
	}
	if c.Tier == Dedicated && !sc.Pinned && sc.Workspace != c.Workspace {
		return false
	}
	return true
}

func (p *cases) stamp(sc *store.Case) {
	if p.c.Region != "" {
		sc.DataRegion = p.c.Region
	}
}

func (p *cases) Put(ctx context.Context, sc store.Case) error {
	p.stamp(&sc)
	if !p.c.Allows(sc) {
		return store.ErrNotFound
	}
	// Never overwrite a case that belongs elsewhere under the same id.
	if old, err := p.inner.Get(ctx, sc.ID); err == nil && !p.c.Allows(old) {
		return store.ErrNotFound
	}
	return p.inner.Put(ctx, sc)
}

func (p *cases) Get(ctx context.Context, id string) (store.Case, error) {
	sc, err := p.inner.Get(ctx, id)
	if err != nil {
		return store.Case{}, err
	}
	if !p.c.Allows(sc) {
		return store.Case{}, store.ErrNotFound
	}
	return sc, nil
}

func (p *cases) Update(ctx context.Context, id string, fn func(*store.Case) error) (store.Case, error) {
	return p.inner.Update(ctx, id, func(sc *store.Case) error {
		if !p.c.Allows(*sc) {
			return store.ErrNotFound
		}
		if err := fn(sc); err != nil {
			return err
		}
		p.stamp(sc)
		if !p.c.Allows(*sc) {
			return store.ErrNotFound
		}
		return nil
	})
}

func (p *cases) List(ctx context.Context) ([]store.Case, error) {
	all, err := p.inner.List(ctx)
	return p.filter(all), err
}

func (p *cases) Delete(ctx context.Context, id string) error {
	if _, err := p.Get(ctx, id); err != nil {
		return err
	}
	return p.inner.Delete(ctx, id)
}

// ListRecent keeps the inner store's own query. A dedicated or regional
// deployment has its own database (docs/design), so the filter rarely drops
// anything and the n newest stay close to n.
func (p *cases) ListRecent(ctx context.Context, n int) ([]store.Case, error) {
	all, err := store.ListRecent(ctx, p.inner, n)
	return p.filter(all), err
}

func (p *cases) ListWorkspace(ctx context.Context, workspace string) ([]store.Case, error) {
	all, err := store.ListWorkspace(ctx, p.inner, workspace)
	return p.filter(all), err
}

func (p *cases) ListOwner(ctx context.Context, owner string, n int) ([]store.Case, error) {
	all, err := store.ListOwner(ctx, p.inner, owner, n)
	return p.filter(all), err
}

func (p *cases) filter(all []store.Case) []store.Case {
	out := all[:0:0]
	for _, sc := range all {
		if p.c.Allows(sc) {
			out = append(out, sc)
		}
	}
	return out
}

var (
	_ store.Cases           = (*cases)(nil)
	_ store.RecentLister    = (*cases)(nil)
	_ store.WorkspaceLister = (*cases)(nil)
	_ store.OwnerLister     = (*cases)(nil)
)
