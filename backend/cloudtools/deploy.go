package cloudtools

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	run "google.golang.org/api/run/v2"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// Changing tools: deploy a build and roll back to a revision. Both only work
// on the services in Client.DeployTargets, and both stop for a person to
// approve before they change anything (ADK tool confirmation: the chat shows
// an Approve / Reject card, and the tool runs again with the decision).

const operationTimeout = 4 * time.Minute

var revisionName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

// ChangeTools returns deploy and rollback, or nothing when no service may be
// deployed to.
func (c *Client) ChangeTools() ([]tool.Tool, error) {
	if len(c.DeployTargets) == 0 {
		return nil, nil
	}
	targets := strings.Join(c.DeployTargets, ", ")
	deploy, err := functiontool.New(functiontool.Config{
		Name: "deploy",
		Description: "Deploys the image a successful Cloud Build produced to a Cloud Run service and sends all traffic to it. " +
			"A person must approve first: calling this shows them an approval card and returns status awaiting_approval; " +
			"it runs when they approve. Services it may deploy to: " + targets + ".",
	}, withApproval(c.planDeploy, c.applyDeploy))
	if err != nil {
		return nil, err
	}
	rollback, err := functiontool.New(functiontool.Config{
		Name: "rollback",
		Description: "Sends all of a Cloud Run service's traffic to an earlier revision. " +
			"A person must approve first, as with deploy. Services it may change: " + targets + ".",
	}, withApproval(c.planRollback, c.applyRollback))
	if err != nil {
		return nil, err
	}
	return []tool.Tool{deploy, rollback}, nil
}

// ChangeResult is what deploy and rollback return. Status is one of
// awaiting_approval, rejected, done or failed.
type ChangeResult struct {
	Status   string    `json:"status"`
	Service  string    `json:"service"`
	Summary  string    `json:"summary"`
	Revision string    `json:"revision,omitempty"`
	Image    string    `json:"image,omitempty"`
	Problem  string    `json:"problem,omitempty"`
	Reason   string    `json:"reason,omitempty"` // why the person rejected it
	Traffic  []traffic `json:"traffic,omitempty"`
}

// Decision is a person's answer on the approval card, sent as the payload of
// a confirmed tool confirmation (confirmed only means "answered").
type Decision struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// decisionFrom reads the payload, which is a Decision when set in-process or
// a decoded JSON map. Anything else counts as rejected.
func decisionFrom(payload any) Decision {
	switch p := payload.(type) {
	case Decision:
		return p
	case *Decision:
		if p != nil {
			return *p
		}
	case map[string]any:
		approved, _ := p["approved"].(bool)
		reason, _ := p["reason"].(string)
		return Decision{Approved: approved, Reason: reason}
	}
	return Decision{}
}

// withApproval builds a tool handler that validates the request (plan), asks
// for approval, and only calls apply once a person has approved. plan runs
// again on approval, so a request that went stale in between is refused.
func withApproval[A any, P any](
	plan func(context.Context, A) (P, ChangeResult, error),
	apply func(context.Context, P, ChangeResult) (ChangeResult, error),
) functiontool.Func[A, ChangeResult] {
	return func(ctx agent.ToolContext, args A) (ChangeResult, error) {
		p, res, err := plan(ctx, args)
		if err != nil {
			return ChangeResult{}, err
		}
		confirmation := ctx.ToolConfirmation()
		if confirmation == nil {
			if err := ctx.RequestConfirmation(res.Summary, nil); err != nil {
				return ChangeResult{}, err
			}
			res.Status = "awaiting_approval"
			return res, nil
		}
		// Unconfirmed is normally answered by ADK before this runs.
		d := decisionFrom(confirmation.Payload)
		if !confirmation.Confirmed || !d.Approved {
			res.Status, res.Reason = "rejected", d.Reason
			return res, nil
		}
		return apply(ctx, p, res)
	}
}

func (c *Client) checkTarget(service string) error {
	if !slices.Contains(c.DeployTargets, service) {
		return fmt.Errorf("not allowed to change %q; the agent may only change: %s", service, strings.Join(c.DeployTargets, ", "))
	}
	return nil
}

func (c *Client) serviceName(service string) string {
	return c.parent() + "/services/" + service
}

type DeployArgs struct {
	Service string `json:"service" jsonschema:"Cloud Run service to deploy to"`
	BuildID string `json:"build_id" jsonschema:"ID of a successful build of this service's image, from list_builds"`
}

type deployPlan struct {
	service, image string
}

func (c *Client) planDeploy(ctx context.Context, args DeployArgs) (deployPlan, ChangeResult, error) {
	if err := c.checkTarget(args.Service); err != nil {
		return deployPlan{}, ChangeResult{}, err
	}
	if !buildID.MatchString(args.BuildID) {
		return deployPlan{}, ChangeResult{}, fmt.Errorf("%q is not a build ID; call list_builds to see them", args.BuildID)
	}
	b, err := c.builds.Projects.Locations.Builds.Get(c.parent() + "/builds/" + args.BuildID).Context(ctx).Do()
	if err != nil {
		return deployPlan{}, ChangeResult{}, fmt.Errorf("get build %s: %w", args.BuildID, err)
	}
	if b.Status != "SUCCESS" {
		return deployPlan{}, ChangeResult{}, fmt.Errorf("build %s has status %s; only successful builds can be deployed", args.BuildID, b.Status)
	}

	// The image this build pushed for the service, pinned to its digest so the
	// deploy can't pick up a different image pushed to the same tag later.
	var image, tag string
	if b.Results != nil {
		for _, img := range b.Results.Images {
			repo, t, _ := strings.Cut(img.Name[strings.LastIndex(img.Name, "/")+1:], ":")
			if repo == args.Service && img.Digest != "" {
				image = img.Name[:strings.LastIndex(img.Name, "/")+1] + repo + "@" + img.Digest
				tag = t
				break
			}
		}
	}
	if image == "" {
		return deployPlan{}, ChangeResult{}, fmt.Errorf("build %s didn't produce an image for %s", args.BuildID, args.Service)
	}

	summary := fmt.Sprintf("Deploy build %s", shortID(args.BuildID))
	if tag != "" && tag != "latest" {
		summary += fmt.Sprintf(" (version %s)", tag)
	}
	summary += fmt.Sprintf(" to %s and send it all traffic", args.Service)
	return deployPlan{service: args.Service, image: image},
		ChangeResult{Service: args.Service, Summary: summary, Image: image}, nil
}

func (c *Client) applyDeploy(ctx context.Context, p deployPlan, res ChangeResult) (ChangeResult, error) {
	svc, err := c.run.Projects.Locations.Services.Get(c.serviceName(p.service)).Context(ctx).Do()
	if err != nil {
		return ChangeResult{}, fmt.Errorf("get service %s: %w", p.service, err)
	}
	if svc.Template == nil || len(svc.Template.Containers) != 1 {
		return ChangeResult{}, fmt.Errorf("service %s must have exactly one container", p.service)
	}
	svc.Template.Containers[0].Image = p.image
	svc.Template.Revision = "" // let Cloud Run name the new revision
	svc.Traffic = []*run.GoogleCloudRunV2TrafficTarget{{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", Percent: 100}}
	return c.update(ctx, svc, res)
}

type RollbackArgs struct {
	Service  string `json:"service" jsonschema:"Cloud Run service to roll back"`
	Revision string `json:"revision" jsonschema:"Earlier revision to send all traffic to, from list_revisions"`
}

func (c *Client) planRollback(ctx context.Context, args RollbackArgs) (RollbackArgs, ChangeResult, error) {
	if err := c.checkTarget(args.Service); err != nil {
		return args, ChangeResult{}, err
	}
	if !revisionName.MatchString(args.Revision) || !strings.HasPrefix(args.Revision, args.Service+"-") {
		return args, ChangeResult{}, fmt.Errorf("%q is not a revision of %s; call list_revisions to see them", args.Revision, args.Service)
	}
	rev, err := c.run.Projects.Locations.Services.Revisions.Get(c.serviceName(args.Service) + "/revisions/" + args.Revision).Context(ctx).Do()
	if err != nil {
		return args, ChangeResult{}, fmt.Errorf("get revision %s: %w", args.Revision, err)
	}
	if !conditionReady(rev.Conditions) {
		return args, ChangeResult{}, fmt.Errorf("revision %s isn't ready, so it can't take traffic", args.Revision)
	}
	return args, ChangeResult{
		Service:  args.Service,
		Revision: args.Revision,
		Summary:  fmt.Sprintf("Roll %s back: send all traffic to %s", args.Service, args.Revision),
	}, nil
}

func (c *Client) applyRollback(ctx context.Context, args RollbackArgs, res ChangeResult) (ChangeResult, error) {
	svc, err := c.run.Projects.Locations.Services.Get(c.serviceName(args.Service)).Context(ctx).Do()
	if err != nil {
		return ChangeResult{}, fmt.Errorf("get service %s: %w", args.Service, err)
	}
	svc.Traffic = []*run.GoogleCloudRunV2TrafficTarget{{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", Revision: args.Revision, Percent: 100}}
	return c.update(ctx, svc, res)
}

// update saves svc, waits for Cloud Run to finish, and reports the result.
// If a new revision fails to start, Cloud Run keeps serving the old one;
// that is reported as failed with the reason.
func (c *Client) update(ctx context.Context, svc *run.GoogleCloudRunV2Service, res ChangeResult) (ChangeResult, error) {
	op, err := c.run.Projects.Locations.Services.Patch(svc.Name, svc).Context(ctx).Do()
	if err != nil {
		return ChangeResult{}, fmt.Errorf("update service %s: %w", res.Service, err)
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	for !op.Done {
		select {
		case <-ctx.Done():
			return ChangeResult{}, fmt.Errorf("update of %s still running after %s; check list_services", res.Service, operationTimeout)
		case <-time.After(c.pollInterval):
		}
		if op, err = c.run.Projects.Locations.Operations.Get(op.Name).Context(ctx).Do(); err != nil {
			return ChangeResult{}, fmt.Errorf("check update of %s: %w", res.Service, err)
		}
	}

	res.Status = "done"
	if op.Error != nil {
		res.Status, res.Problem = "failed", op.Error.Message
	}
	after, err := c.run.Projects.Locations.Services.Get(svc.Name).Context(ctx).Do()
	if err != nil {
		return ChangeResult{}, fmt.Errorf("get service %s: %w", res.Service, err)
	}
	if res.Revision == "" {
		res.Revision = lastPart(after.LatestCreatedRevision)
	}
	res.Traffic = trafficOf(after)
	if res.Status == "done" && after.LatestReadyRevision != after.LatestCreatedRevision {
		res.Status = "failed"
		res.Problem = "new revision didn't become ready; the previous revision is still serving"
	}
	return res, nil
}

func conditionReady(conds []*run.GoogleCloudRunV2Condition) bool {
	for _, c := range conds {
		if c.Type == "Ready" {
			return c.State == "CONDITION_SUCCEEDED"
		}
	}
	return false
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
