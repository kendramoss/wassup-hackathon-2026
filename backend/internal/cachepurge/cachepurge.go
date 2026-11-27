// Package cachepurge opens the case-to-cache index and the stores it points
// into from the server's configuration, so the server and cmd/retention agree
// on where everything is (#875, CACHE_PURGE).
package cachepurge

import (
	"context"
	"os"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/blob"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/config"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

// Setup is the index and the raw stores (no encryption layer is needed to
// delete). Refs is nil when the verifier cache is off.
type Setup struct {
	Refs   blob.Refs
	Stores map[string]blob.Store
}

// Purger removes what a case alone used.
func (s Setup) Purger() blob.Purger { return blob.Purger{Refs: s.Refs, Stores: s.Stores} }

// Open reads cfg. fs is the Firestore client when the caller has one (nil: one
// is opened when the cache is in Firestore).
func Open(ctx context.Context, cfg config.Config, fs *store.Firestore) (Setup, error) {
	switch cfg.VerifierCacheStore {
	case config.VerdictsFirestore:
		if fs == nil {
			var err error
			if fs, err = store.NewFirestore(ctx, cfg.GCPProject, cfg.FirestoreDatabase); err != nil {
				return Setup{}, err
			}
		}
		return Setup{Refs: fs.CacheRefs(), Stores: map[string]blob.Store{
			blob.StoreVerdicts: fs.Verdicts(), blob.StoreEngine: fs.Blobs(store.EngineCollection)}}, nil
	case config.VerdictsFile:
		var refs blob.Refs = blob.NewMemoryRefs()
		if cfg.CasesStore == config.CasesFile && cfg.CasesDir != "" {
			r, err := blob.NewFileRefs(cfg.CasesDir)
			if err != nil {
				return Setup{}, err
			}
			refs = r
		}
		engine := filepath.Join(filepath.Dir(filepath.Clean(cfg.VerifierCacheDir)), "engine-cache")
		_ = os.MkdirAll(cfg.VerifierCacheDir, 0o755)
		return Setup{Refs: refs, Stores: map[string]blob.Store{
			blob.StoreVerdicts: blob.Dir{Dir: cfg.VerifierCacheDir}, blob.StoreEngine: blob.Dir{Dir: engine}}}, nil
	}
	return Setup{}, nil
}
