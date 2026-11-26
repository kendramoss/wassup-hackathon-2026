package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/envelope"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/placement"
)

// infraSettings are the application side of the deployment-tier, residency,
// CMEK and status-page work (#872, #873, #874, #885). All are off by
// default; the infrastructure each needs is in docs/design/.
type infraSettings struct {
	placement  placement.Config
	blobKey    envelope.Provider // nil: blobs stored as before
	statusPage bool              // STATUS_PAGE=on serves GET /v1/status
}

func newInfra(ctx context.Context, getenv func(string) string, logger *slog.Logger) (infraSettings, error) {
	var s infraSettings
	var errs []error
	p, err := placement.FromEnv(getenv)
	errs = append(errs, err)
	s.placement = p
	ec, err := envelope.ConfigFromEnv(getenv)
	errs = append(errs, err)
	switch v := strings.ToLower(strings.TrimSpace(getenv("STATUS_PAGE"))); v {
	case "", "off":
	case "on":
		s.statusPage = true
	default:
		errs = append(errs, fmt.Errorf("STATUS_PAGE must be on or off, not %q", v))
	}
	if err := errors.Join(errs...); err != nil {
		return s, err
	}
	if s.blobKey, err = ec.Provider(ctx); err != nil {
		return s, err
	}
	if p.Active() {
		logger.Info("placement guard on", "tier", string(p.Tier))
	}
	if s.blobKey != nil {
		logger.Info("cache blobs encrypted", "encryption", string(ec.Mode))
	}
	return s, nil
}
