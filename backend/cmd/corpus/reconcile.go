package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/corpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// Generated-output ownership (#317). A command that writes facts files also
// records which ruling ids it wrote in a manifest inside the same directory.
// On the next run only ids named by the previous manifest, and no longer
// produced, are removed, one exact <id>.txt each. Files the manifest does not
// name (separately generated Part 102 facts, user files, legacy output from
// before manifests) are never touched, and nothing is deleted by glob or
// recursively.
const (
	buildManifest  = ".generated_manifest.json"
	bundleManifest = ".bundle_manifest.json"
	stageSuffix    = ".stage"
)

var factsFileName = regexp.MustCompile(`^[NH][0-9]{6}\.txt$`)

type manifest struct {
	Version int      `json:"version"`
	IDs     []string `json:"ids"`
}

// readManifest returns the ids named by dir/name. A missing manifest is
// (nil, false, nil); an unreadable, malformed or invalid one is an error so
// the caller stops before changing anything.
func readManifest(dir, name string) ([]string, bool, error) {
	p := filepath.Join(dir, name)
	st, err := os.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !st.Mode().IsRegular() {
		return nil, false, fmt.Errorf("manifest %s is not a regular file", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, false, err
	}
	var m manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, false, fmt.Errorf("manifest %s: %w", p, err)
	}
	if m.Version != 1 {
		return nil, false, fmt.Errorf("manifest %s: unsupported version %d", p, m.Version)
	}
	for _, id := range m.IDs {
		if !domain.ValidRulingID(id) {
			return nil, false, fmt.Errorf("manifest %s: invalid ruling id %q", p, id)
		}
	}
	return m.IDs, true, nil
}

// legacyOwned derives the owned ids of a directory written before manifests.
// Provable evidence is the split_meta.jsonl a previous build wrote beside its
// facts files (one line per ruling it generated). Without that evidence, any
// ruling-shaped facts file that is neither produced now nor protected might
// be stale generated output, so we refuse rather than leave it loaded.
func legacyOwned(dir, metaName string, facts map[string][]byte, protect map[string]bool) ([]string, bool, error) {
	if metaName != "" {
		p := filepath.Join(dir, metaName)
		st, err := os.Lstat(p)
		switch {
		case err == nil:
			if !st.Mode().IsRegular() {
				return nil, false, fmt.Errorf("%s is not a regular file", p)
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return nil, false, err
			}
			var ids []string
			for i, ln := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				if ln == "" {
					continue
				}
				var m struct {
					Ruling string `json:"ruling"`
				}
				if err := json.Unmarshal([]byte(ln), &m); err != nil || !domain.ValidRulingID(m.Ruling) {
					return nil, false, fmt.Errorf("%s line %d is corrupt; fix or delete it, then rerun (nothing was changed)", p, i+1)
				}
				ids = append(ids, m.Ruling)
			}
			return ids, true, nil
		case !errors.Is(err, fs.ErrNotExist):
			return nil, false, err
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, false, err
	}
	var unexplained []string
	for _, e := range ents {
		n := e.Name()
		if !factsFileName.MatchString(n) || !e.Type().IsRegular() {
			continue
		}
		id := strings.TrimSuffix(n, ".txt")
		if _, ok := facts[id]; ok || protect[id] {
			continue
		}
		unexplained = append(unexplained, n)
	}
	if len(unexplained) > 0 {
		sort.Strings(unexplained)
		return nil, false, fmt.Errorf("%s has no manifest or previous split_meta, so ownership of %d ruling file(s) cannot be proven (e.g. %s) and they might be stale generated facts that corpus.Load would still use; nothing was changed. Move them away or restore the previous split_meta.jsonl, then rerun", dir, len(unexplained), unexplained[0])
	}
	return nil, false, nil
}

func manifestBytes(ids map[string]bool) []byte {
	m := manifest{Version: 1, IDs: make([]string, 0, len(ids))}
	for id := range ids {
		m.IDs = append(m.IDs, id)
	}
	sort.Strings(m.IDs)
	b, _ := json.MarshalIndent(m, "", " ")
	return append(b, '\n')
}

// reconcileResult reports what a reconcile did.
type reconcileResult struct {
	Written  int
	Pruned   []string
	Skipped  []string // owned but obsolete files left alone (not a regular file)
	Unmanned bool     // no manifest and no legacy evidence: nothing could be pruned
	// Bootstrapped: ownership came from a pre-manifest split_meta.jsonl.
	Bootstrapped bool
}

// reconcile writes facts (id -> content as <id>.txt) and extras (exact file
// name -> content) into dir and removes the obsolete files the previous
// manifest owned. Ids in protect are never removed. Everything is staged
// before anything is replaced, so a failure while staging leaves the last
// good output untouched.
func reconcile(dir, manifestName, legacyMeta string, facts map[string][]byte, extras map[string][]byte, protect map[string]bool) (res reconcileResult, err error) {
	for id := range facts {
		if !domain.ValidRulingID(id) {
			return res, fmt.Errorf("refusing ruling id %q: not a CBP ruling number", id)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	old, found, err := readManifest(dir, manifestName)
	if err != nil {
		return res, err
	}
	if !found {
		// Legacy directory: bootstrap ownership only from provable evidence.
		old, found, err = legacyOwned(dir, legacyMeta, facts, protect)
		if err != nil {
			return res, err
		}
		res.Bootstrapped = found
	}
	res.Unmanned = !found
	var obsolete []string
	for _, id := range old {
		if _, keep := facts[id]; !keep && !protect[id] {
			obsolete = append(obsolete, id)
		}
	}
	sort.Strings(obsolete)

	// Stage every output next to its destination.
	type staged struct{ tmp, dst string }
	var stage []staged
	defer func() {
		if err != nil {
			for _, s := range stage {
				_ = os.Remove(s.tmp) // our own tmp names only
			}
		}
	}()
	stageFile := func(name string, b []byte) error {
		dst := filepath.Join(dir, name)
		tmp := dst + stageSuffix
		if err := os.WriteFile(tmp, b, 0o644); err != nil {
			return err
		}
		stage = append(stage, staged{tmp, dst})
		return nil
	}
	names := make([]string, 0, len(facts))
	for id := range facts {
		names = append(names, id)
	}
	sort.Strings(names)
	for _, id := range names {
		if err := stageFile(id+".txt", facts[id]); err != nil {
			return res, err
		}
	}
	extraNames := make([]string, 0, len(extras))
	for n := range extras {
		extraNames = append(extraNames, n)
	}
	sort.Strings(extraNames)
	for _, n := range extraNames {
		if n != filepath.Base(n) || n == "." || n == ".." || n == "" {
			return res, fmt.Errorf("refusing extra file name %q", n)
		}
		if err := stageFile(n, extras[n]); err != nil {
			return res, err
		}
	}

	// Commit. The interim manifest names old and new ids, so a crash after
	// this point leaves every file we may have created or must still prune
	// owned, and a rerun finishes the job.
	union := map[string]bool{}
	for _, id := range old {
		union[id] = true
	}
	for id := range facts {
		union[id] = true
	}
	if err = writeAtomic(filepath.Join(dir, manifestName), manifestBytes(union)); err != nil {
		return res, err
	}
	for _, s := range stage {
		if err = os.Rename(s.tmp, s.dst); err != nil {
			return res, err
		}
	}
	stage = nil
	res.Written = len(facts)

	for _, id := range obsolete {
		p := filepath.Join(dir, id+".txt") // id validated by readManifest
		st, lerr := os.Lstat(p)
		if errors.Is(lerr, fs.ErrNotExist) {
			continue
		}
		if lerr != nil {
			return res, lerr
		}
		if !st.Mode().IsRegular() { // symlink, directory, device: not ours to delete
			res.Skipped = append(res.Skipped, id)
			continue
		}
		if err = os.Remove(p); err != nil {
			return res, err
		}
		res.Pruned = append(res.Pruned, id)
	}

	final := map[string]bool{}
	for id := range facts {
		final[id] = true
	}
	for _, id := range res.Skipped { // still on disk, still ours
		final[id] = true
	}
	err = writeAtomic(filepath.Join(dir, manifestName), manifestBytes(final))
	return res, err
}

func writeAtomic(path string, b []byte) error {
	tmp := path + stageSuffix
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// protectedIDs reads the ruling ids of a labels file whose facts are
// generated separately (usmca-label); a missing file protects nothing.
func protectedIDs(path string) (map[string]bool, error) {
	if path == "" {
		return nil, nil
	}
	if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	ls, err := corpus.LoadLabels(path)
	if err != nil {
		return nil, err
	}
	m := map[string]bool{}
	for _, l := range ls {
		m[l.Ruling] = true
	}
	return m, nil
}

// bundle copies the facts files of a built corpus into the directory baked
// into the image and removes the files a previous bundle copied that the
// source no longer has.
func bundle(args []string) error {
	fl := flag.NewFlagSet("bundle", flag.ExitOnError)
	from := fl.String("from", "../data/corpus/facts", "built facts directory")
	to := fl.String("to", "data/facts", "bundled facts directory")
	keep := fl.String("keep-labels", "../eval/data/labels_usmca.jsonl", "labels file whose facts are bundled separately (Part 102); never pruned")
	_ = fl.Parse(args)
	protect, err := protectedIDs(*keep)
	if err != nil {
		return err
	}
	ents, err := os.ReadDir(*from)
	if err != nil {
		return err
	}
	facts := map[string][]byte{}
	for _, e := range ents {
		if !factsFileName.MatchString(e.Name()) || !e.Type().IsRegular() {
			continue // manifests, split_meta, symlinks and strays are not bundled
		}
		b, err := os.ReadFile(filepath.Join(*from, e.Name()))
		if err != nil {
			return err
		}
		facts[e.Name()[:len(e.Name())-len(".txt")]] = b
	}
	res, err := reconcile(*to, bundleManifest, "", facts, nil, protect)
	if err != nil {
		return err
	}
	reportReconcile("bundle", *to, res)
	return nil
}

func reportReconcile(cmd, dir string, res reconcileResult) {
	fmt.Fprintf(os.Stderr, "%s: %d facts files in %s, %d obsolete removed\n", cmd, res.Written, dir, len(res.Pruned))
	if res.Bootstrapped {
		fmt.Fprintf(os.Stderr, "%s: no manifest in %s; ownership bootstrapped from the previous build's split_meta.jsonl\n", cmd, dir)
	}
	for _, id := range res.Skipped {
		fmt.Fprintf(os.Stderr, "%s: left %s.txt alone (not a regular file)\n", cmd, id)
	}
}
