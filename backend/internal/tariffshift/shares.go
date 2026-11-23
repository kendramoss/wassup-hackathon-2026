package tariffshift

import (
	"fmt"
	"math"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/domain"
)

// ValidateShares checks that every material's optional ValueShare and
// WeightShare is a finite fraction in [0, 1], the range the API documents. A
// nil share is "not given" and is accepted; 0 and 1 are accepted. It does not
// restrict the sum of the shares. The error wraps domain.ErrInvalid.
func ValidateShares(materials []Material) error {
	check := func(i int, field string, v *float64) error {
		if v == nil {
			return nil
		}
		if math.IsNaN(*v) || math.IsInf(*v, 0) || *v < 0 || *v > 1 {
			return fmt.Errorf("%w: materials[%d].%s must be a number from 0 to 1", domain.ErrInvalid, i, field)
		}
		return nil
	}
	for i, m := range materials {
		if err := check(i, "value_share", m.ValueShare); err != nil {
			return err
		}
		if err := check(i, "weight_share", m.WeightShare); err != nil {
			return err
		}
	}
	return nil
}
