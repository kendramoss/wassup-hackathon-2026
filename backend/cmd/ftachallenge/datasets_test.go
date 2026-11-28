package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

const challengeDir = "../../../eval/challenges/fta"

func TestChallengeDatasetsAreValid(t *testing.T) {
	dev, test, err := loadAll(challengeDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range features {
		if n := len(test[f].Cases); n < MinTestCases {
			t.Errorf("%s: %d reserved test cases", f, n)
		}
		shared, only := FamilyOverlap(dev[f], test[f])
		if len(shared) == 0 || len(only) == 0 {
			t.Errorf("%s: development and test should share some scenario families and keep some test-only (shared %d, test-only %d)", f, len(shared), len(only))
		}
	}
}

func TestOriginalEvidenceIsUntouched(t *testing.T) {
	pinned := map[string]string{
		"atiga/reference-cases.json":    "643375e9ac7a0197fb4d231ec664729a4ae976ccd2f9e454d09ed0c822664459",
		"atiga/evaluation-cases.json":   "22871769ec54988be2c235c3d6511c63c24b07382033036b68fc4f9810fd0202",
		"atiga/evaluation-results.json": "5c42b2baf6f6d41e82f0f813a32ffd676b90b3ecba98957aca6a6b7cb1ad73da",
		"akfta/reference-cases.json":    "c674a1b0055f444ca4e87855790a525e15df301ef59eb8e6b7eb0257945a8554",
		"akfta/post-hoc-cases.json":     "cc50b8cd07bd7dc3a2fa5917f490e855489626d4799f0865d3fea7b3d0448d6b",
		"acfta/reference-cases.json":    "607050ae3520d7119291375075c6daf1808e71783f8ef8a73c10e4a00401a7b5",
		"acfta/post-hoc-cases.json":     "fc6bb403da86e62890192bd26f0939ce275032bda8310660e33d5ec331b4ab05",
		"rcep/japan-dev.jsonl":          "819517d9c38b13bb73b182366d8979bfa0584a83735d9a1b3f45adcd501d09eb",
		"rcep/japan-heldout.jsonl":      "f98f2362887b7827b3dffcc0d0ce050312a19e0d7dc480480494c0beece05e6c",
		"rcep/japan-summary.json":       "a8653a4c3d41f34c0c890a78720112fd318c1721937f1f06746ebec8e722a1c6",
	}
	for rel, want := range pinned {
		b, err := os.ReadFile(filepath.Join("../../../eval", rel))
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != want {
			t.Errorf("%s changed: %s", rel, got)
		}
	}
}
