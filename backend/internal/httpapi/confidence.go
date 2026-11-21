package httpapi

import (
	"fmt"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// confidencePhrase reports a prediction's confidence the way the Result screen
// and MCP replies do: the calibrated probability first, with the model's own
// value labelled when the map applied. Without a map the value is labelled as
// stated. It keeps the Ask agent, history summaries and BOM-version summaries
// from giving two unexplained numbers for one prediction (issue #402, D161).
func confidencePhrase(p domain.Prediction) string {
	if p.CalibratedConfidence != nil {
		return fmt.Sprintf("calibrated %.2f; model stated %.2f", *p.CalibratedConfidence, p.Confidence)
	}
	return fmt.Sprintf("confidence %.2f as stated by the model, not calibrated", p.Confidence)
}
