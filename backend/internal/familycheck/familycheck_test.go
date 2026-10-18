package familycheck

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/llm"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/planner"
)

func checker(t *testing.T, answer string) (*Checker, *llm.Request) {
	t.Helper()
	tmpls, err := planner.LoadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	var seen llm.Request
	m := &llm.Fake{Respond: func(req llm.Request) (llm.Response, error) {
		seen = req
		return llm.Response{Text: answer}, nil
	}}
	return New(m, "gemini-3.8-flash", tmpls), &seen
}

func TestCheckMatchesATemplate(t *testing.T) {
	c, req := checker(t, `{"family":"motorised_tools_small_appliances","confidence":1.4,"reason":"Motor, fan and heating element."}`)
	r, err := c.Check(context.Background(), "hair dryer")
	if err != nil {
		t.Fatal(err)
	}
	if r.Family != "motorised_tools_small_appliances" || r.Name == "" || r.Confidence != 1 {
		t.Errorf("result = %+v", r)
	}
	enum := req.JSONSchema["properties"].(map[string]any)["family"].(map[string]any)["enum"].([]string)
	if len(enum) < 4 || enum[len(enum)-1] != None || !strings.Contains(req.System, "led_lights") {
		t.Errorf("schema enum = %v", enum)
	}
}

func TestCheckNone(t *testing.T) {
	c, _ := checker(t, `{"family":"none","confidence":0.8,"reason":"A sofa shares no groups."}`)
	if r, err := c.Check(context.Background(), "sofa"); err != nil || r.Family != None || r.Name != "" {
		t.Errorf("none = %+v, %v", r, err)
	}
}

func TestCheckRejectsAnInventedFamily(t *testing.T) {
	c, _ := checker(t, `{"family":"furniture","confidence":0.9,"reason":"x"}`)
	if _, err := c.Check(context.Background(), "sofa"); !errors.Is(err, llm.ErrBadResponse) {
		t.Errorf("err = %v", err)
	}
}
