package mcpserver

import "github.com/google/jsonschema-go/jsonschema"

// Small builders for the tools' input schemas. Every object is closed
// (additionalProperties false) and every string and array is capped, so a
// client gets a schema error before any API call is made.

func ptr[T any](v T) *T { return &v }

func str(desc string, minLen, maxLen int) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string", Description: desc, MaxLength: ptr(maxLen)}
	if minLen > 0 {
		s.MinLength = ptr(minLen)
	}
	return s
}

func pattern(desc, re string, maxLen int) *jsonschema.Schema {
	s := str(desc, 1, maxLen)
	s.Pattern = re
	return s
}

func enum(desc string, vals ...string) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "string", Description: desc}
	for _, v := range vals {
		s.Enum = append(s.Enum, v)
	}
	return s
}

func integer(desc string, lo, hi float64) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "integer", Description: desc, Minimum: ptr(lo), Maximum: ptr(hi)}
}

func array(desc string, items *jsonschema.Schema, minItems, maxItems int) *jsonschema.Schema {
	s := &jsonschema.Schema{Type: "array", Description: desc, Items: items, MaxItems: ptr(maxItems)}
	if minItems > 0 {
		s.MinItems = ptr(minItems)
	}
	return s
}

func object(desc string, required []string, props map[string]*jsonschema.Schema) *jsonschema.Schema {
	return &jsonschema.Schema{Type: "object", Description: desc, Required: required, Properties: props,
		AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
}

// Shared field schemas.
const (
	countryPattern = `^[A-Za-z][A-Za-z .,'()-]{1,59}$`
	datePattern    = `^[0-9]{4}-[0-9]{2}-[0-9]{2}$`
	// A case id is "case_" and 12 hex characters; older stored ids differ,
	// so only the character set and length are checked here.
	caseIDPattern = `^[A-Za-z0-9_-]{1,64}$`
	// A case access key is the anonymous caller's visitor ID (16 to 128 of
	// these characters), the same shape the API accepts.
	accessKeyPattern = `^[A-Za-z0-9_-]{16,128}$`
)

func country(desc string) *jsonschema.Schema { return pattern(desc, countryPattern, 60) }

func caseID() *jsonschema.Schema {
	return pattern("The case id predict_origin returned (case_...), or a demo case id from list_cases.", caseIDPattern, 64)
}

func accessKey() *jsonschema.Schema {
	return pattern("Only without an API key: the case_access_key predict_origin returned. It is a secret that grants access to the cases made with it; omit it when the connection has an API key.", accessKeyPattern, 128)
}

func (t tools) family() *jsonschema.Schema {
	if len(t.families) == 0 {
		return str("Product family (template) id, e.g. motorised_tools_small_appliances.", 1, 64)
	}
	return enum("Product family. Needed for plan search on a structured product; also guides extraction.", t.families...)
}

func (t tools) product() *jsonschema.Schema {
	// Each use needs its own schema value: a schema must be a tree.
	id := func() *jsonschema.Schema { return str("", 1, 64) }
	name := func() *jsonschema.Schema { return str("", 1, 200) }
	plant := object("A plant (factory) in the supply chain.", []string{"id", "country"}, map[string]*jsonschema.Schema{
		"id":           str("Short id other rows refer to, e.g. cn1.", 1, 64),
		"name":         name(),
		"country":      country("Country the plant is in."),
		"capabilities": array("Operation groups this plant could perform (from the family template); plan search only moves groups to plants that list them.", id(), 0, 40),
	})
	component := object("A part and where it is made.", []string{"name", "made_in"}, map[string]*jsonschema.Schema{
		"name":    name(),
		"made_in": country("Country the part is made in."),
		"plant":   id(),
		"group":   id(),
	})
	op := object("An assembly or processing step and the plant that does it.", []string{"name", "plant"}, map[string]*jsonschema.Schema{
		"name":  name(),
		"plant": id(),
		"group": id(),
	})
	return object("A structured product: parts, plants and operations. extract_product drafts one from text.", []string{"name", "components"}, map[string]*jsonschema.Schema{
		"name":        name(),
		"family":      t.family(),
		"description": str("", 0, 4000),
		"hts":         pattern("Optional U.S. HTS subheading, e.g. 8419.89.95.", `^[0-9][0-9. -]{7,13}$`, 14),
		"plants":      array("", plant, 0, 50),
		"components":  array("", component, 1, 300),
		"operations":  array("", op, 0, 300),
	})
}
