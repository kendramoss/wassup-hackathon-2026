package cloudtools

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

const (
	operationTimeout = 4 * time.Minute
	healthAttempts   = 3
)

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
		Description: "Deploys the image a successful Cloud Build produced to a Cloud Run service: starts it with no traffic, checks its health, " +
			"then sends it all traffic, or rolls back automatically (status rolled_back) if the health check fails. " +
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
// awaiting_approval, rejected, done, failed, or rolled_back (deploy only: the
// new revision failed its health check and never got traffic).
type ChangeResult struct {
	Status   string    `json:"status"`
	Service  string    `json:"service"`
	Summary  string    `json:"summary"`
	Revision string    `json:"revision,omitempty"`
	Image    string    `json:"image,omitempty"`
	Problem  string    `json:"problem,omitempty"`
	Reason   string    `json:"reason,omitempty"` // why the person rejected it
	Steps    []string  `json:"steps,omitempty"`  // what deploy did, in order
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
	summary += fmt.Sprintf(" to %s; it gets traffic only if its health check passes", args.Service)
	return deployPlan{service: args.Service, image: image},
		ChangeResult{Service: args.Service, Summary: summary, Image: image}, nil
}

// applyDeploy rolls the build out so that a bad version never gets user
// traffic:
//
//  1. create a revision with the new image and no traffic (what's live keeps
//     serving);
//  2. give it a tag, so it has its own URL, and check HealthPath there;
//  3. healthy: send it all traffic. Unhealthy: remove the tag and report
//     rolled_back; users never saw it.
func (c *Client) applyDeploy(ctx context.Context, p deployPlan, res ChangeResult) (ChangeResult, error) {
	svc, err := c.run.Projects.Locations.Services.Get(c.serviceName(p.service)).Context(ctx).Do()
	if err != nil {
		return ChangeResult{}, fmt.Errorf("get service %s: %w", p.service, err)
	}
	if svc.Template == nil || len(svc.Template.Containers) != 1 {
		return ChangeResult{}, fmt.Errorf("service %s must have exactly one container", p.service)
	}
	serving := pinTraffic(svc)
	live := servingNames(serving)

	// 1. New revision, no traffic.
	svc.Template.Containers[0].Image = p.image
	svc.Template.Revision = "" // let Cloud Run name the new revision
	svc.Traffic = serving
	svc, problem, err := c.save(ctx, svc)
	if err != nil {
		return ChangeResult{}, err
	}
	res.Revision = lastPart(svc.LatestCreatedRevision)
	if problem == "" {
		// The revision's own status: the service's LatestReadyRevision isn't
		// updated for a revision that has no traffic.
		problem, err = c.revisionProblem(ctx, svc.LatestCreatedRevision)
		if err != nil {
			return ChangeResult{}, err
		}
	}
	if problem != "" {
		res.Status, res.Problem = "failed", problem
		res.Steps = append(res.Steps, fmt.Sprintf("Started revision %s with no traffic: it failed to start", res.Revision),
			fmt.Sprintf("Traffic stayed on %s", live))
		res.Traffic = trafficOf(svc)
		return res, nil
	}
	res.Steps = append(res.Steps, fmt.Sprintf("Started revision %s with no traffic; %s kept serving", res.Revision, live))

	// 2. Tag it and check its health on the tag URL.
	svc.Traffic = append(serving, &run.GoogleCloudRunV2TrafficTarget{
		Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", Revision: res.Revision, Percent: 0, Tag: candidateTag,
	})
	if svc, problem, err = c.save(ctx, svc); err != nil {
		return ChangeResult{}, err
	}
	healthy, detail := false, problem
	if problem == "" {
		healthy, detail = c.checkHealth(ctx, tagURL(svc, candidateTag))
	}
	res.Steps = append(res.Steps, fmt.Sprintf("Health check %s on %s: %s", c.HealthPath, res.Revision, detail))

	// 3. Promote, or drop the tag and leave traffic where it was.
	if !healthy {
		svc.Traffic = serving
		if svc, problem, err = c.save(ctx, svc); err != nil {
			return ChangeResult{}, err
		}
		res.Status, res.Problem = "rolled_back", "health check failed: "+detail
		res.Steps = append(res.Steps, fmt.Sprintf("Rolled back: traffic stayed on %s; %s never got any", live, res.Revision))
		if problem != "" {
			res.Steps = append(res.Steps, "Removing the health-check tag failed: "+problem)
		}
		res.Traffic = trafficOf(svc)
		return res, nil
	}
	// By name, not "latest": Cloud Run resolves latest to LatestReadyRevision,
	// which hasn't moved to a revision that had no traffic.
	svc.Traffic = []*run.GoogleCloudRunV2TrafficTarget{{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", Revision: res.Revision, Percent: 100}}
	if svc, problem, err = c.save(ctx, svc); err != nil {
		return ChangeResult{}, err
	}
	res.Traffic = trafficOf(svc)
	if problem != "" {
		res.Status, res.Problem = "failed", "healthy, but switching traffic failed: "+problem
		return res, nil
	}
	res.Status = "done"
	res.Steps = append(res.Steps, fmt.Sprintf("Sent all traffic to %s", res.Revision))
	return res, nil
}

const candidateTag = "candidate"

// pinTraffic returns svc's current traffic as explicit revisions (resolving
// "latest", dropping tags and 0% targets), so creating a new revision
// doesn't move it.
func pinTraffic(svc *run.GoogleCloudRunV2Service) []*run.GoogleCloudRunV2TrafficTarget {
	var out []*run.GoogleCloudRunV2TrafficTarget
	for _, t := range trafficOf(svc) {
		if t.Percent > 0 && t.Revision != "" {
			out = append(out, &run.GoogleCloudRunV2TrafficTarget{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", Revision: t.Revision, Percent: t.Percent})
		}
	}
	if len(out) == 0 && svc.LatestReadyRevision != "" {
		out = append(out, &run.GoogleCloudRunV2TrafficTarget{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", Revision: lastPart(svc.LatestReadyRevision), Percent: 100})
	}
	return out
}

func servingNames(ts []*run.GoogleCloudRunV2TrafficTarget) string {
	var names []string
	for _, t := range ts {
		names = append(names, t.Revision)
	}
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}

func tagURL(svc *run.GoogleCloudRunV2Service, tag string) string {
	for _, t := range svc.TrafficStatuses {
		if t.Tag == tag {
			return t.Uri
		}
	}
	return ""
}

// checkHealth GETs HealthPath on baseURL a few times, until it answers 2xx.
// It returns whether it did and a short description of the last answer.
func (c *Client) checkHealth(ctx context.Context, baseURL string) (bool, string) {
	if baseURL == "" {
		return false, "the new revision has no URL to check"
	}
	detail := ""
	for attempt := range healthAttempts {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return false, detail
			case <-time.After(c.healthRetryDelay):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+c.HealthPath, nil)
		if err != nil {
			return false, err.Error()
		}
		resp, err := c.http.Do(req)
		if err != nil {
			detail = "no answer (" + err.Error() + ")"
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		resp.Body.Close()
		detail = fmt.Sprintf("HTTP %d %s", resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return true, detail
		}
	}
	return false, detail
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

// update saves a traffic change (rollback) and reports the result.
func (c *Client) update(ctx context.Context, svc *run.GoogleCloudRunV2Service, res ChangeResult) (ChangeResult, error) {
	after, problem, err := c.save(ctx, svc)
	if err != nil {
		return ChangeResult{}, err
	}
	res.Status, res.Problem = "done", problem
	if problem != "" {
		res.Status = "failed"
	}
	res.Traffic = trafficOf(after)
	return res, nil
}

// revisionProblem returns why revision (a full resource name) isn't ready,
// or "" if it is.
func (c *Client) revisionProblem(ctx context.Context, revision string) (string, error) {
	rev, err := c.run.Projects.Locations.Services.Revisions.Get(revision).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("get revision %s: %w", lastPart(revision), err)
	}
	for _, cond := range rev.Conditions {
		if cond.Type == "Ready" && cond.State != "CONDITION_SUCCEEDED" {
			if cond.Message != "" {
				return "the new revision didn't start: " + cond.Message, nil
			}
			return "the new revision didn't start", nil
		}
	}
	if !conditionReady(rev.Conditions) {
		return "the new revision didn't start", nil
	}
	return "", nil
}

// save patches svc, waits for Cloud Run to finish, and returns the service
// as it is afterwards. problem is set when Cloud Run reports the change
// failed; err when the API calls themselves fail.
func (c *Client) save(ctx context.Context, svc *run.GoogleCloudRunV2Service) (after *run.GoogleCloudRunV2Service, problem string, err error) {
	service := lastPart(svc.Name)
	op, err := c.run.Projects.Locations.Services.Patch(svc.Name, svc).Context(ctx).Do()
	if err != nil {
		return nil, "", fmt.Errorf("update service %s: %w", service, err)
	}
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	for !op.Done {
		select {
		case <-ctx.Done():
			return nil, "", fmt.Errorf("update of %s still running after %s; check list_services", service, operationTimeout)
		case <-time.After(c.pollInterval):
		}
		if op, err = c.run.Projects.Locations.Operations.Get(op.Name).Context(ctx).Do(); err != nil {
			return nil, "", fmt.Errorf("check update of %s: %w", service, err)
		}
	}
	if op.Error != nil {
		problem = op.Error.Message
	}
	after, err = c.run.Projects.Locations.Services.Get(svc.Name).Context(ctx).Do()
	if err != nil {
		return nil, "", fmt.Errorf("get service %s: %w", service, err)
	}
	return after, problem, nil
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
