package retention

import (
	"context"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/audit"
)

// AuditRecorder records each purge deletion as a system case.delete event
// (detail "retention") in the workspace's tenant. Nil svc: no recorder.
func AuditRecorder(svc *audit.Service) Recorder {
	if svc == nil {
		return nil
	}
	return func(ctx context.Context, t Tombstone) error {
		obj, _ := audit.ObjectRef("case", t.CaseID)
		_, err := svc.Record(ctx, audit.Draft{Event: audit.Event{Tenant: "ws:" + t.Workspace, ActorKind: audit.ActorSystem,
			Action: audit.CaseDelete, Phase: audit.PhaseSingle, Outcome: audit.Allowed, Detail: "retention", Object: obj}})
		return err
	}
}
