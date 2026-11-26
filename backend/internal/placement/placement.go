// Package placement holds where a deployment's case data may live and whom
// the deployment serves (#872 tenant isolation, #873 data residency).
//
// Both are off by default: with TENANT_TIER unset (shared) and DATA_REGION
// unset, Wrap returns the case store unchanged and nothing differs from a
// deployment without this package. The infrastructure side (a separate
// Cloud Run service, Firestore database and region per tenant or region) is
// the owner's; see docs/design/tenant-isolation.md and
// docs/design/data-residency.md. This package is the application's guard
// that a deployment never reads or writes a case outside its placement,
// for example after a misrouted request or a copied database.
package placement

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Tier is how a deployment is shared between customers.
type Tier string

const (
	// Shared serves every workspace (today's behaviour).
	Shared Tier = "shared"
	// Dedicated serves one workspace only.
	Dedicated Tier = "dedicated"
)

// Config is a deployment's placement.
type Config struct {
	Tier Tier
	// Workspace is the one team workspace a dedicated deployment serves.
	Workspace string
	// Region is the data region this deployment's case data belongs to
	// ("us", "eu", ...); empty means no residency tag (today's behaviour).
	Region string
}

var regionRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}$`)

// FromEnv reads TENANT_TIER, TENANT_WORKSPACE and DATA_REGION.
func FromEnv(getenv func(string) string) (Config, error) {
	c := Config{
		Tier:      Tier(strings.ToLower(strings.TrimSpace(getenv("TENANT_TIER")))),
		Workspace: strings.TrimSpace(getenv("TENANT_WORKSPACE")),
		Region:    strings.ToLower(strings.TrimSpace(getenv("DATA_REGION"))),
	}
	if c.Tier == "" {
		c.Tier = Shared
	}
	var errs []error
	switch c.Tier {
	case Shared:
		if c.Workspace != "" {
			errs = append(errs, errors.New("TENANT_WORKSPACE is set but TENANT_TIER is not dedicated"))
		}
	case Dedicated:
		if c.Workspace == "" {
			errs = append(errs, errors.New("TENANT_TIER=dedicated needs TENANT_WORKSPACE"))
		}
	default:
		errs = append(errs, fmt.Errorf("TENANT_TIER must be shared or dedicated, not %q", c.Tier))
	}
	if c.Region != "" && !regionRE.MatchString(c.Region) {
		errs = append(errs, fmt.Errorf("DATA_REGION %q must be a short lowercase name such as us or eu", c.Region))
	}
	return c, errors.Join(errs...)
}

// Active reports whether the placement changes anything.
func (c Config) Active() bool { return c.Tier == Dedicated || c.Region != "" }
