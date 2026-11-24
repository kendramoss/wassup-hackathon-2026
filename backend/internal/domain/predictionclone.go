package domain

import "encoding/json"

// Clone copies p deeply (slices, pointers and nested results), so a kept
// copy shares nothing with the prediction it was taken from. It round-trips
// through JSON, the form the stores keep it in; on the impossible error it
// falls back to a shallow copy with its slices copied.
func (p Prediction) Clone() Prediction {
	b, err := json.Marshal(p)
	if err == nil {
		var out Prediction
		if json.Unmarshal(b, &out) == nil {
			return out
		}
	}
	out := p
	out.KeyFactors = append([]string(nil), p.KeyFactors...)
	out.CitedRulings = append([]string(nil), p.CitedRulings...)
	out.Precedents = append([]string(nil), p.Precedents...)
	return out
}
