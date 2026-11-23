package mcpserver

import (
	"strings"
	"testing"
)

func TestUSMCASchemaAndDescription(t *testing.T) {
	s := usmcaSchema()
	if s.AdditionalProperties == nil {
		t.Error("unknown properties are allowed")
	}
	for _, k := range []string{"destination", "regime", "good_hs", "rvc_method", "transaction_value", "net_cost", "vnm", "materials", "profile"} {
		if s.Properties[k] == nil {
			t.Errorf("missing %q", k)
		}
	}
	for _, k := range []string{"transaction_value", "net_cost", "vnm"} {
		if s.Properties[k].Type != "string" {
			t.Errorf("%s must be a string, got %q", k, s.Properties[k].Type)
		}
	}
	if m := s.Properties["materials"]; m.MaxItems == nil || *m.MaxItems != 200 {
		t.Errorf("materials cap = %v", m.MaxItems)
	}
	for _, want := range []string{"NOT a certification", "NOT a tariff rate", "legal origin status", "is a known zero", "unknown", "selected-rule status only"} {
		if !strings.Contains(usmcaDescription, want) {
			t.Errorf("description lacks %q", want)
		}
	}
}
