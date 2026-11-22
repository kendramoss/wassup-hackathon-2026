package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
)

func bundle(args []string) error {
	fs := flag.NewFlagSet("bundle", flag.ExitOnError)
	check := fs.Bool("check", false, "fail if the embedded snapshot differs")
	_ = fs.Parse(args)

	c, err := jpcorpus.Load(dataDir)
	if err != nil {
		return err
	}
	sp, err := jpcorpus.LoadSplits(filepath.Join(dataDir, "splits.json"))
	if err != nil {
		return err
	}
	var b bytes.Buffer
	if err := jpcorpus.WriteBundle(&b, c, append(sp.Dev, sp.HeldOut...)); err != nil {
		return err
	}
	out := "internal/jpcorpus/data/precedents.json.gz"
	if *check {
		got, err := os.ReadFile(out)
		if err != nil {
			return err
		}
		if !bytes.Equal(got, b.Bytes()) {
			return fmt.Errorf("%s differs from the Japan corpus; re-run jpeval bundle", out)
		}
		fmt.Println("jp precedent snapshot ok")
		return nil
	}
	return os.WriteFile(out, b.Bytes(), 0o644)
}
