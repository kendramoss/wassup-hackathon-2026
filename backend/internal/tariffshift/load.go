package tariffshift

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

//go:embed rules.json
var rulesJSON []byte

var (
	defaultOnce  sync.Once
	defaultRules *Rules
	defaultErr   error
)

// Default returns the committed rule table (rules.json).
func Default() (*Rules, error) {
	defaultOnce.Do(func() {
		var t Table
		if err := json.Unmarshal(rulesJSON, &t); err != nil {
			defaultErr = fmt.Errorf("tariffshift: rules.json: %w", err)
			return
		}
		defaultRules = New(t)
	})
	return defaultRules, defaultErr
}
