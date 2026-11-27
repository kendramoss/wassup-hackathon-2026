// Command textilerules builds and evaluates the 19 CFR 102.21 rule check
// (D951).
//
//	go run ./cmd/textilerules rules      # rebuild internal/textilerules/rules.json
//	go run ./cmd/textilerules show 6104  # the rows and alternatives for a code
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/textilerules"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: textilerules rules|show|extract|eval|relabel|labelcheck|build-third")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "rules":
		err = cmdRules()
	case "show":
		err = cmdShow(os.Args[2:])
	case "extract":
		err = cmdExtract(os.Args[2:])
	case "eval":
		err = cmdEval(os.Args[2:])
	case "build-third":
		err = cmdBuildThird(os.Args[2:])
	case "labelcheck":
		err = cmdLabelCheck(os.Args[2:])
	case "relabel":
		err = cmdRelabel(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "textilerules:", err)
		os.Exit(1)
	}
}

const rulesFile = "internal/textilerules/rules.json"

func cmdRules() error {
	ecfr, fr, err := textilerules.SourceFiles()
	if err != nil {
		return err
	}
	t, err := textilerules.Build(ecfr, fr, textilerules.SourceInfo)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(t, "", " ")
	if err := os.WriteFile(rulesFile, append(b, '\n'), 0o644); err != nil {
		return err
	}
	alts, unmodeled := 0, 0
	for _, r := range t.Rows {
		for _, a := range r.Alts {
			alts++
			for _, c := range a.Conds {
				if c.Unmodeled != "" {
					unmodeled++
					break
				}
			}
		}
	}
	fmt.Printf("%d rows, %d alternatives (%d with a condition the facts do not give), %d scope entries\n", len(t.Rows), alts, unmodeled, len(t.Scope))
	return nil
}

func cmdShow(args []string) error {
	r, err := textilerules.Default()
	if err != nil {
		return err
	}
	for _, row := range r.T.Rows {
		if len(args) > 0 && !textilerules.RowMatches(row, args[0]) {
			continue
		}
		fmt.Printf("%s (%s)\n", row.Code, row.Origin)
		for _, a := range row.Alts {
			b, _ := json.Marshal(struct {
				ID    string
				After string `json:",omitempty"`
				Conds []textilerules.Cond
				Req   textilerules.Req
			}{a.ID, a.After, a.Conds, a.Req})
			fmt.Printf("  %s\n", b)
		}
	}
	return nil
}
