package nporigin

import "strings"

// EvidenceFor names the document or input that would settle a need (#770,
// D800). It reads only the need's kind and key, so the same wording comes
// back from every route and tool. A review need gets "": it is a person's
// reading of rule text, which no document settles, so it is never asked as
// a question.
func EvidenceFor(n Need) string {
	switch n.Kind {
	case NeedReview:
		return ""
	case NeedRow:
		return "The product specification or catalogue description, to match the good to one of the listed descriptions"
	case NeedShare:
		if n.Key == "weight_share" {
			return "The bill of materials with each material's net weight and the good's net weight"
		}
		return "The supplier's bill of materials with values, and the good's ex-works price"
	case NeedFact:
		switch {
		case strings.HasSuffix(n.Key, ".mix"):
			return "The formula or recipe sheet showing how the materials are mixed"
		case strings.HasSuffix(n.Key, ".r"):
			return "A production record showing the good was made from blanks, and where"
		}
		return "A production record from the plant showing where this work was done"
	}
	switch n.Key {
	case "good_code":
		return "The HS code on the commercial invoice or the good's classification"
	case "material_hts":
		return "The HS code on the supplier's invoice or the supplier's declaration"
	case "material_origin":
		return "The supplier's declaration or certificate of origin"
	case "country":
		return "The production record or plant address for the last stage of production"
	case "materials":
		return "The bill of materials, each line with its HS code and origin"
	}
	return ""
}
