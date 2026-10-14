// Package cloudtools gives the agent read-only tools to see what's running on
// Cloud Run and how it got there (Cloud Build). Everything is scoped to one
// project and region.
package cloudtools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"google.golang.org/api/cloudbuild/v1"
	"google.golang.org/api/option"
	run "google.golang.org/api/run/v2"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	defaultLimit = 5
	maxLimit     = 20
)

// Cloud Run service names: lowercase letters, digits and hyphens.
var serviceName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,48}$`)

// Client calls the Cloud Run Admin and Cloud Build APIs for one project and
// region.
type Client struct {
	project, region string
	run             *run.Service
	builds          *cloudbuild.Service
}

// New creates a Client. opts are passed to both API clients (tests use them
// to point at a fake server).
func New(ctx context.Context, project, region string, opts ...option.ClientOption) (*Client, error) {
	r, err := run.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("cloud run client: %w", err)
	}
	b, err := cloudbuild.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("cloud build client: %w", err)
	}
	return &Client{project: project, region: region, run: r, builds: b}, nil
}

func (c *Client) parent() string {
	return fmt.Sprintf("projects/%s/locations/%s", c.project, c.region)
}

// Tools returns the agent tools backed by this client.
func (c *Client) Tools() ([]tool.Tool, error) {
	var tools []tool.Tool
	var errs []error
	add := func(t tool.Tool, err error) {
		tools = append(tools, t)
		errs = append(errs, err)
	}
	add(functiontool.New(functiontool.Config{
		Name:        "list_services",
		Description: "Lists the Cloud Run services in this project: URL, latest ready revision, and how traffic is split between revisions.",
	}, withContext(c.ListServices)))
	add(functiontool.New(functiontool.Config{
		Name:        "list_revisions",
		Description: "Lists a Cloud Run service's most recent revisions, newest first, with image, creation time, readiness and traffic share.",
	}, withContext(c.ListRevisions)))
	add(functiontool.New(functiontool.Config{
		Name:        "list_builds",
		Description: "Lists recent Cloud Build runs, newest first, with status, timing and the image each one built.",
	}, withContext(c.ListBuilds)))
	add(functiontool.New(functiontool.Config{
		Name:        "get_build",
		Description: "Returns one Cloud Build run in detail: status, each step's status and the failure reason.",
	}, withContext(c.GetBuild)))
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return tools, nil
}

// withContext adapts a method taking a plain context to a tool handler.
func withContext[A, R any](f func(context.Context, A) (R, error)) functiontool.Func[A, R] {
	return func(ctx agent.ToolContext, args A) (R, error) { return f(ctx, args) }
}

// lastPart returns the text after the final "/", e.g. a revision name from
// its full resource name.
func lastPart(s string) string {
	return s[strings.LastIndex(s, "/")+1:]
}

func clampLimit(n int) int {
	if n <= 0 {
		return defaultLimit
	}
	return min(n, maxLimit)
}

type traffic struct {
	Revision string `json:"revision"`
	Percent  int64  `json:"percent"`
	Tag      string `json:"tag,omitempty"`
}

type serviceInfo struct {
	Name                string    `json:"name"`
	URL                 string    `json:"url"`
	LatestReadyRevision string    `json:"latest_ready_revision"`
	Ready               bool      `json:"ready"`
	Problem             string    `json:"problem,omitempty"`
	Traffic             []traffic `json:"traffic"`
	Updated             string    `json:"updated"`
}

type ListServicesArgs struct{}

type ListServicesResult struct {
	Services []serviceInfo `json:"services"`
}

// trafficOf resolves "latest" targets to the revision they point at.
func trafficOf(s *run.GoogleCloudRunV2Service) []traffic {
	var out []traffic
	for _, t := range s.TrafficStatuses {
		rev := lastPart(t.Revision)
		if rev == "" && t.Type == "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST" {
			rev = lastPart(s.LatestReadyRevision)
		}
		out = append(out, traffic{Revision: rev, Percent: t.Percent, Tag: t.Tag})
	}
	return out
}

func (c *Client) ListServices(ctx context.Context, _ ListServicesArgs) (ListServicesResult, error) {
	resp, err := c.run.Projects.Locations.Services.List(c.parent()).Context(ctx).Do()
	if err != nil {
		return ListServicesResult{}, fmt.Errorf("list services: %w", err)
	}
	out := ListServicesResult{Services: []serviceInfo{}}
	for _, s := range resp.Services {
		info := serviceInfo{
			Name:                lastPart(s.Name),
			URL:                 s.Uri,
			LatestReadyRevision: lastPart(s.LatestReadyRevision),
			Traffic:             trafficOf(s),
			Updated:             s.UpdateTime,
		}
		if tc := s.TerminalCondition; tc != nil {
			info.Ready = tc.State == "CONDITION_SUCCEEDED"
			if !info.Ready {
				info.Problem = tc.Message
			}
		}
		out.Services = append(out.Services, info)
	}
	return out, nil
}

type revisionInfo struct {
	Name           string `json:"name"`
	Created        string `json:"created"`
	Image          string `json:"image"`
	Ready          bool   `json:"ready"`
	Problem        string `json:"problem,omitempty"`
	TrafficPercent int64  `json:"traffic_percent"`
}

type ListRevisionsArgs struct {
	Service string `json:"service" jsonschema:"Cloud Run service name, e.g. backend"`
	Limit   int    `json:"limit,omitempty" jsonschema:"How many revisions to return, newest first. Default 5, max 20."`
}

type ListRevisionsResult struct {
	Service   string         `json:"service"`
	Revisions []revisionInfo `json:"revisions"`
}

func (c *Client) ListRevisions(ctx context.Context, args ListRevisionsArgs) (ListRevisionsResult, error) {
	if !serviceName.MatchString(args.Service) {
		return ListRevisionsResult{}, fmt.Errorf("%q is not a Cloud Run service name; call list_services to see the names", args.Service)
	}
	svc, err := c.run.Projects.Locations.Services.Get(c.parent() + "/services/" + args.Service).Context(ctx).Do()
	if err != nil {
		return ListRevisionsResult{}, fmt.Errorf("get service %s: %w", args.Service, err)
	}
	percent := map[string]int64{}
	for _, t := range trafficOf(svc) {
		percent[t.Revision] += t.Percent
	}

	var revs []*run.GoogleCloudRunV2Revision
	err = c.run.Projects.Locations.Services.Revisions.List(svc.Name).Context(ctx).Pages(ctx, func(r *run.GoogleCloudRunV2ListRevisionsResponse) error {
		revs = append(revs, r.Revisions...)
		return nil
	})
	if err != nil {
		return ListRevisionsResult{}, fmt.Errorf("list revisions of %s: %w", args.Service, err)
	}
	// RFC 3339 timestamps in UTC sort as strings.
	sort.Slice(revs, func(i, j int) bool { return revs[i].CreateTime > revs[j].CreateTime })

	out := ListRevisionsResult{Service: args.Service, Revisions: []revisionInfo{}}
	for _, r := range revs[:min(len(revs), clampLimit(args.Limit))] {
		info := revisionInfo{Name: lastPart(r.Name), Created: r.CreateTime, TrafficPercent: percent[lastPart(r.Name)]}
		if len(r.Containers) > 0 {
			info.Image = r.Containers[0].Image
		}
		for _, cond := range r.Conditions {
			if cond.Type == "Ready" {
				info.Ready = cond.State == "CONDITION_SUCCEEDED"
				if !info.Ready {
					info.Problem = cond.Message
				}
			}
		}
		out.Revisions = append(out.Revisions, info)
	}
	return out, nil
}

type buildInfo struct {
	ID       string   `json:"id"`
	Status   string   `json:"status"`
	Created  string   `json:"created"`
	Started  string   `json:"started,omitempty"`
	Finished string   `json:"finished,omitempty"`
	Images   []string `json:"images,omitempty"`
	Failure  string   `json:"failure,omitempty"`
}

func toBuildInfo(b *cloudbuild.Build) buildInfo {
	info := buildInfo{
		ID: b.Id, Status: b.Status, Created: b.CreateTime, Started: b.StartTime, Finished: b.FinishTime,
		Images: b.Images,
	}
	if b.FailureInfo != nil {
		info.Failure = strings.TrimSpace(b.FailureInfo.Type + ": " + b.FailureInfo.Detail)
	} else if b.Status != "SUCCESS" {
		info.Failure = b.StatusDetail
	}
	return info
}

type ListBuildsArgs struct {
	Service string `json:"service,omitempty" jsonschema:"Only builds of this Cloud Run service's image. Leave empty for all builds."`
	Limit   int    `json:"limit,omitempty" jsonschema:"How many builds to return, newest first. Default 5, max 20."`
}

type ListBuildsResult struct {
	Builds []buildInfo `json:"builds"`
}

func (c *Client) ListBuilds(ctx context.Context, args ListBuildsArgs) (ListBuildsResult, error) {
	if args.Service != "" && !serviceName.MatchString(args.Service) {
		return ListBuildsResult{}, fmt.Errorf("%q is not a Cloud Run service name; call list_services to see the names", args.Service)
	}
	limit := clampLimit(args.Limit)
	// Builds come newest first. When filtering by service, fetch a wider page
	// and filter here: source deploys don't tag builds with the service.
	pageSize := int64(limit)
	if args.Service != "" {
		pageSize = 50
	}
	resp, err := c.builds.Projects.Locations.Builds.List(c.parent()).PageSize(pageSize).Context(ctx).Do()
	if err != nil {
		return ListBuildsResult{}, fmt.Errorf("list builds: %w", err)
	}
	out := ListBuildsResult{Builds: []buildInfo{}}
	for _, b := range resp.Builds {
		if args.Service != "" && !buildsService(b, args.Service) {
			continue
		}
		out.Builds = append(out.Builds, toBuildInfo(b))
		if len(out.Builds) == limit {
			break
		}
	}
	return out, nil
}

// buildsService reports whether b built an image named after service, e.g.
// .../cloud-run-source-deploy/backend:latest or .../backend@sha256:...
func buildsService(b *cloudbuild.Build, service string) bool {
	for _, img := range b.Images {
		name := lastPart(img)
		if i := strings.IndexAny(name, ":@"); i >= 0 {
			name = name[:i]
		}
		if name == service {
			return true
		}
	}
	return false
}

type buildStep struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type GetBuildArgs struct {
	ID string `json:"id" jsonschema:"Build ID, as returned by list_builds"`
}

type GetBuildResult struct {
	buildInfo
	Steps []buildStep `json:"steps"`
}

var buildID = regexp.MustCompile(`^[0-9a-f-]{8,64}$`)

func (c *Client) GetBuild(ctx context.Context, args GetBuildArgs) (GetBuildResult, error) {
	if !buildID.MatchString(args.ID) {
		return GetBuildResult{}, fmt.Errorf("%q is not a build ID; call list_builds to see them", args.ID)
	}
	b, err := c.builds.Projects.Locations.Builds.Get(c.parent() + "/builds/" + args.ID).Context(ctx).Do()
	if err != nil {
		return GetBuildResult{}, fmt.Errorf("get build %s: %w", args.ID, err)
	}
	out := GetBuildResult{buildInfo: toBuildInfo(b), Steps: []buildStep{}}
	for _, s := range b.Steps {
		name := s.Id
		if name == "" {
			name = s.Name
		}
		out.Steps = append(out.Steps, buildStep{Name: name, Status: s.Status})
	}
	return out, nil
}
