package main

import (
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/jpcorpus"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/rcep"
)

func TestInputsUsePublishedFactsOnly(t *testing.T) {
	r := ruling{
		HS: "4412.31", Process: "ベトナム国内において、加工する",
		Reasoning: "中国で生産する。マレーシア原産材料と認められる。",
		Inputs: []jpcorpus.Input{
			{NameJA: "木材", TextJA: "木材（中国製）", Headings: []string{"44.08"}, Countries: []string{"China"}},
			{NameJA: "接着剤", TextJA: "接着剤（タイ原産）", Chapters: []int{35}, Headings: []string{"35.06"}, Countries: []string{"Thailand"}},
			{NameJA: "その他", Subheadings: []string{"4412.31", "4412.39"}},
		},
	}
	in := inputOf(r)
	if in.ProducedIn != "Vietnam" || in.FOB != nil || in.WhollyObtained || len(in.Operations) != 0 {
		t.Fatalf("unexpected ruling input: %+v", in)
	}
	if in.Materials[0].HS != "4408" || in.Materials[0].Status != rcep.StatusUnknown ||
		in.Materials[1].HS != "3506" || in.Materials[1].Status != rcep.StatusOriginating || in.Materials[2].HS != "" {
		t.Fatalf("unexpected material evidence: %+v", in.Materials)
	}
}

func TestSyntheticOnlyChangesFirstNonOriginatingCode(t *testing.T) {
	in := rcep.Input{HS: "4412.31", ProducedIn: "Vietnam", Materials: []rcep.Material{
		{HS: "4407", Country: "Thailand", Status: rcep.StatusOriginating},
		{HS: "4408", Status: rcep.StatusUnknown},
	}}
	result := calculate(in)
	if result.Status != rcep.Yes || result.Criterion != "CTH" {
		t.Fatalf("original case: %+v", result)
	}
	mutant := counterexample(in, result)
	if mutant == nil || mutant.MaterialIndex != 1 || mutant.ReplacementHS != "4412" || mutant.Result.Status != rcep.Undetermined {
		t.Fatalf("synthetic case: %+v", mutant)
	}
	if in.Materials[1].HS != "4408" || in.Materials[1].Value != nil {
		t.Fatalf("source inputs changed: %+v", in.Materials)
	}
}
