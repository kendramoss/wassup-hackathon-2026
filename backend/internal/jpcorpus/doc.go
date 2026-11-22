// Package jpcorpus loads the Japan Customs advance rulings on origin
// (data/jp, D710) and indexes them for precedent retrieval.
//
// The Japanese text as published is the record; English translations
// (data/jp/translations-en.jsonl) exist for retrieval and display only and
// are never a label. Labels come from data/jp/labels-*.jsonl, which
// eval/jp/labels.py derives from the Japanese fields.
//
// The BM25 index mirrors the U.S. one (retrieval.NewBM25 over English facts,
// strictly-earlier precedents only), so a Japan engine can use the same
// retrieval seam once the jurisdiction registry (J1) is in place.
package jpcorpus
