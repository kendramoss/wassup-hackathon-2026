package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/familycheck"
)

// FamilyChecker matches typed text to a template family (familycheck.Checker).
type FamilyChecker interface {
	Check(ctx context.Context, text string) (familycheck.Result, error)
}

const maxFamilyText = 2000

// familyCheck answers POST /v1/products/family-check {text, product?}: the
// supported family the product belongs to, or "none" (18 Oct). The product's
// name and parts, when given, are read with the text. Nothing is saved.
func (a *api) familyCheck(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Text    string          `json:"text"`
		Product *domain.Product `json:"product"`
	}
	if err := decode(w, r, &in); err != nil {
		a.fail(w, r, err)
		return
	}
	text := strings.TrimSpace(in.Text)
	if in.Product != nil {
		var parts []string
		for _, c := range in.Product.Components {
			if n := strings.TrimSpace(c.Name); n != "" {
				parts = append(parts, n)
			}
		}
		if n := strings.TrimSpace(in.Product.Name); n != "" {
			text += "\nProduct name: " + n
		}
		if len(parts) > 0 {
			text += "\nParts: " + strings.Join(parts, ", ")
		}
	}
	text = strings.TrimSpace(text)
	if text == "" || utf8.RuneCountInString(text) > maxFamilyText {
		a.fail(w, r, fmt.Errorf("%w: text is required, up to %d characters", domain.ErrInvalid, maxFamilyText))
		return
	}
	res, err := a.d.Family.Check(r.Context(), text)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": res, "prompt_version": familycheck.PromptVersion, "llm_provider": a.d.Provider})
}
