package main

import (
	"encoding/json"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMediaChallengeStudyReproducesWithoutModelCalls(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for offline study reproduction")
	}
	if err := filepath.Walk("../../../eval/challenges/media", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		_, err = os.ReadFile(path)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-I", "-B", "../../../eval/challenges/media/test_study.py"},
		{"-I", "-B", "../../../eval/challenges/media/study.py", "summarize", "--check"},
	} {
		output, err := exec.Command(python, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("offline media study: %v\n%s", err, output)
		}
	}
}

func TestRunRetainsMissingCountryWarnings(t *testing.T) {
	t.Setenv("LLM_PROVIDER", "fake")
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "a.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 800, 600))); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`[{"image":"a.png","source":"test","components":[{"name":"electric motor","country":"China"}]}]`), 0600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "result.json")
	if err := run(dir, out, "", ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Scores []struct {
			Warnings  []string `json:"warnings"`
			Simulated bool     `json:"simulated"`
		} `json:"scores"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Scores) != 1 || !result.Scores[0].Simulated {
		t.Fatal("expected one explicitly simulated test score")
	}
	if !strings.Contains(strings.Join(result.Scores[0].Warnings, " "), "power cord") {
		t.Fatal("missing-country warning was discarded")
	}
}
