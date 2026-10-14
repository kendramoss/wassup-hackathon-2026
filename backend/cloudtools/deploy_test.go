package cloudtools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/api/option"
	run "google.golang.org/api/run/v2"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/tool/toolconfirmation"
)

const demoImage = "us-central1-docker.pkg.dev/p/cloud-run-source-deploy/demo-app"

// fakeRun is a small stateful Cloud Run + Cloud Build API: PATCH on the
// service creates a new revision (or only changes traffic), and the
// operation finishes on the first poll. Tagged revisions get a URL on the
// same server whose /health answers 500 for revisions in unhealthy.
type fakeRun struct {
	mu        sync.Mutex
	svc       run.GoogleCloudRunV2Service
	revs      int
	failNext  bool // the next new revision won't become ready
	failed    map[string]bool
	unhealthy map[string]bool
	patches   int
	traffic   []string // every traffic split saved, e.g. "demo-app-00001=100 demo-app-00002=0(candidate)"
}

func newFakeRun(t *testing.T) (*Client, *fakeRun) {
	t.Helper()
	f := &fakeRun{revs: 1, unhealthy: map[string]bool{}, failed: map[string]bool{}, svc: run.GoogleCloudRunV2Service{
		Name:                  parent + "/services/demo-app",
		LatestCreatedRevision: parent + "/services/demo-app/revisions/demo-app-00001",
		LatestReadyRevision:   parent + "/services/demo-app/revisions/demo-app-00001",
		Template:              &run.GoogleCloudRunV2RevisionTemplate{Containers: []*run.GoogleCloudRunV2Container{{Image: demoImage + "@sha256:old"}}},
		TrafficStatuses:       []*run.GoogleCloudRunV2TrafficTargetStatus{{Type: "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", Percent: 100}},
	}}
	builds := map[string]string{
		"aaaaaaaa-0001": `{"id": "aaaaaaaa-0001", "status": "SUCCESS", "results": {"images": [
			{"name": "` + demoImage + `:abc1234", "digest": "sha256:new"}]}}`,
		"bbbbbbbb-0002": `{"id": "bbbbbbbb-0002", "status": "FAILURE"}`,
		"cccccccc-0003": `{"id": "cccccccc-0003", "status": "SUCCESS", "results": {"images": [
			{"name": "us-central1-docker.pkg.dev/p/cloud-run-source-deploy/backend:latest", "digest": "sha256:x"}]}}`,
	}
	svcPath := "/v2/" + f.svc.Name

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		switch {
		case strings.HasPrefix(r.URL.Path, "/tag/") && strings.HasSuffix(r.URL.Path, "/health"):
			if f.unhealthy[strings.Split(r.URL.Path, "/")[2]] {
				http.Error(w, "unhealthy", http.StatusInternalServerError)
				return
			}
			io.WriteString(w, "ok")
			return
		case strings.HasPrefix(r.URL.Path, "/v1/"+parent+"/builds/"):
			if b, ok := builds[lastPart(r.URL.Path)]; ok {
				io.WriteString(w, b)
				return
			}
		case r.URL.Path == svcPath && r.Method == http.MethodGet:
			enc.Encode(f.svc)
			return
		case r.URL.Path == svcPath && r.Method == http.MethodPatch:
			f.patches++
			var in run.GoogleCloudRunV2Service
			json.NewDecoder(r.Body).Decode(&in)
			if in.Template.Containers[0].Image != f.svc.Template.Containers[0].Image {
				f.revs++
				rev := parent + "/services/demo-app/revisions/demo-app-0000" + string(rune('0'+f.revs))
				// Like Cloud Run: LatestReadyRevision only moves when the new
				// revision is ready and gets traffic (see below).
				f.svc.LatestCreatedRevision = rev
				if f.failNext {
					f.failed[lastPart(rev)] = true
				}
			}
			f.svc.Template = in.Template
			f.svc.TrafficStatuses = nil
			var split []string
			for _, t := range in.Traffic {
				st := &run.GoogleCloudRunV2TrafficTargetStatus{Type: t.Type, Revision: t.Revision, Percent: t.Percent, Tag: t.Tag}
				rev := t.Revision
				if rev == "" {
					rev = lastPart(f.svc.LatestReadyRevision)
				}
				entry := fmt.Sprintf("%s=%d", rev, t.Percent)
				if t.Tag != "" {
					st.Uri = srv.URL + "/tag/" + rev
					entry += "(" + t.Tag + ")"
				}
				f.svc.TrafficStatuses = append(f.svc.TrafficStatuses, st)
				split = append(split, entry)
			}
			f.traffic = append(f.traffic, strings.Join(split, " "))
			for _, t := range in.Traffic {
				if t.Percent > 0 && (t.Revision == lastPart(f.svc.LatestCreatedRevision) || t.Type == "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST") && !f.failed[lastPart(f.svc.LatestCreatedRevision)] {
					f.svc.LatestReadyRevision = f.svc.LatestCreatedRevision
				}
			}
			io.WriteString(w, `{"name": "`+parent+`/operations/op1", "done": false}`)
			return
		case r.URL.Path == "/v2/"+parent+"/operations/op1":
			io.WriteString(w, `{"name": "`+parent+`/operations/op1", "done": true}`)
			return
		case strings.HasPrefix(r.URL.Path, svcPath+"/revisions/"):
			name := lastPart(r.URL.Path)
			n := 0
			fmt.Sscanf(name, "demo-app-%d", &n)
			switch {
			case n < 1 || n > f.revs:
			case f.failed[name]:
				io.WriteString(w, `{"name": "`+name+`", "conditions": [{"type": "Ready", "state": "CONDITION_FAILED", "message": "container failed to start"}]}`)
				return
			default:
				io.WriteString(w, `{"name": "`+name+`", "conditions": [{"type": "Ready", "state": "CONDITION_SUCCEEDED"}]}`)
				return
			}
		}
		http.Error(w, `{"error": {"code": 404, "message": "not found"}}`, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	c, err := New(context.Background(), "p", "r", option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	c.DeployTargets = []string{"demo-app"}
	c.pollInterval = time.Millisecond
	c.healthRetryDelay = 0
	return c, f
}

// toolCtx is the part of agent.ToolContext the approval flow uses.
type toolCtx struct {
	agent.ToolContext
	context.Context
	confirmation *toolconfirmation.ToolConfirmation
	requested    string
}

func (c *toolCtx) ToolConfirmation() *toolconfirmation.ToolConfirmation { return c.confirmation }
func (c *toolCtx) RequestConfirmation(hint string, _ any) error         { c.requested = hint; return nil }
func (c *toolCtx) Deadline() (time.Time, bool)                          { return c.Context.Deadline() }
func (c *toolCtx) Done() <-chan struct{}                                { return c.Context.Done() }
func (c *toolCtx) Err() error                                           { return c.Context.Err() }
func (c *toolCtx) Value(key any) any                                    { return c.Context.Value(key) }

func TestDeployNeedsApproval(t *testing.T) {
	c, f := newFakeRun(t)
	deploy := withApproval(c.planDeploy, c.applyDeploy)
	args := DeployArgs{Service: "demo-app", BuildID: "aaaaaaaa-0001"}

	// First call: asks for approval and changes nothing.
	ctx := &toolCtx{Context: context.Background()}
	res, err := deploy(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "awaiting_approval" || f.patches != 0 {
		t.Fatalf("status = %q, patches = %d; want awaiting_approval and no change", res.Status, f.patches)
	}
	if want := "Deploy build aaaaaaaa (version abc1234) to demo-app; it gets traffic only if its health check passes"; ctx.requested != want {
		t.Errorf("approval hint = %q, want %q", ctx.requested, want)
	}

	// Rejected (answered but not approved, as a Decision or a decoded JSON
	// payload): no change, reason passed back.
	for _, payload := range []any{Decision{Reason: "not today"}, map[string]any{"approved": false, "reason": "not today"}} {
		ctx = &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true, Payload: payload}}
		res, _ = deploy(ctx, args)
		if res.Status != "rejected" || res.Reason != "not today" || f.patches != 0 {
			t.Fatalf("rejected: %+v, patches = %d", res, f.patches)
		}
	}
	// No decision at all counts as rejected.
	ctx = &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true}}
	if res, _ = deploy(ctx, args); res.Status != "rejected" || f.patches != 0 {
		t.Fatalf("no payload: %+v, patches = %d", res, f.patches)
	}

	// Approved: deploys the build's image by digest and sends it all traffic.
	ctx = &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true, Payload: Decision{Approved: true}}}
	res, err = deploy(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "done" || res.Revision != "demo-app-00002" || res.Image != demoImage+"@sha256:new" {
		t.Fatalf("approved: %+v", res)
	}
	if len(res.Traffic) != 1 || res.Traffic[0] != (traffic{"demo-app-00002", 100, ""}) {
		t.Errorf("traffic = %+v", res.Traffic)
	}
	// New revision first got no traffic, then a tag for the health check,
	// then everything.
	want := []string{
		"demo-app-00001=100",
		"demo-app-00001=100 demo-app-00002=0(candidate)",
		"demo-app-00002=100",
	}
	if strings.Join(f.traffic, " | ") != strings.Join(want, " | ") {
		t.Errorf("traffic history:\n got  %q\n want %q", f.traffic, want)
	}
	if len(res.Steps) != 3 || !strings.Contains(res.Steps[1], "HTTP 200 ok") {
		t.Errorf("steps = %q", res.Steps)
	}
}

func TestDeployRollsBackWhenHealthCheckFails(t *testing.T) {
	c, f := newFakeRun(t)
	f.unhealthy["demo-app-00002"] = true
	ctx := &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true, Payload: Decision{Approved: true}}}
	res, err := withApproval(c.planDeploy, c.applyDeploy)(ctx, DeployArgs{Service: "demo-app", BuildID: "aaaaaaaa-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "rolled_back" || !strings.Contains(res.Problem, "HTTP 500 unhealthy") {
		t.Fatalf("result = %+v, want rolled_back with the health check answer", res)
	}
	// The bad revision never got traffic, and the tag is gone again.
	want := []string{
		"demo-app-00001=100",
		"demo-app-00001=100 demo-app-00002=0(candidate)",
		"demo-app-00001=100",
	}
	if strings.Join(f.traffic, " | ") != strings.Join(want, " | ") {
		t.Errorf("traffic history:\n got  %q\n want %q", f.traffic, want)
	}
	if len(res.Traffic) != 1 || res.Traffic[0] != (traffic{"demo-app-00001", 100, ""}) {
		t.Errorf("traffic = %+v", res.Traffic)
	}
}

func TestDeployRefusesBadRequests(t *testing.T) {
	c, f := newFakeRun(t)
	deploy := withApproval(c.planDeploy, c.applyDeploy)
	for _, args := range []DeployArgs{
		{Service: "backend", BuildID: "cccccccc-0003"},  // not a deploy target
		{Service: "demo-app", BuildID: "bbbbbbbb-0002"}, // failed build
		{Service: "demo-app", BuildID: "cccccccc-0003"}, // build of another service
		{Service: "demo-app", BuildID: "../../x"},       // not an ID
	} {
		// Refused before asking anyone to approve.
		ctx := &toolCtx{Context: context.Background()}
		if _, err := deploy(ctx, args); err == nil || ctx.requested != "" {
			t.Errorf("%+v: err = %v, approval requested = %q; want error and no request", args, err, ctx.requested)
		}
	}
	if f.patches != 0 {
		t.Errorf("patches = %d, want 0", f.patches)
	}
}

func TestDeployReportsRevisionThatFailsToStart(t *testing.T) {
	c, f := newFakeRun(t)
	f.failNext = true
	ctx := &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true, Payload: map[string]any{"approved": true}}}
	res, err := withApproval(c.planDeploy, c.applyDeploy)(ctx, DeployArgs{Service: "demo-app", BuildID: "aaaaaaaa-0001"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "failed" || !strings.Contains(res.Problem, "container failed to start") {
		t.Errorf("result = %+v, want failed with the revision's reason", res)
	}
	if len(f.traffic) != 1 || f.traffic[0] != "demo-app-00001=100" {
		t.Errorf("traffic history = %q, want only the pinned old revision", f.traffic)
	}
}

func TestRollback(t *testing.T) {
	c, f := newFakeRun(t)
	rollback := withApproval(c.planRollback, c.applyRollback)

	ctx := &toolCtx{Context: context.Background(), confirmation: &toolconfirmation.ToolConfirmation{Confirmed: true, Payload: Decision{Approved: true}}}
	res, err := rollback(ctx, RollbackArgs{Service: "demo-app", Revision: "demo-app-00001"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "done" || len(res.Traffic) != 1 || res.Traffic[0] != (traffic{"demo-app-00001", 100, ""}) {
		t.Errorf("result = %+v", res)
	}

	for _, args := range []RollbackArgs{
		{Service: "backend", Revision: "backend-00001"},   // not a deploy target
		{Service: "demo-app", Revision: "backend-00001"},  // another service's revision
		{Service: "demo-app", Revision: "demo-app-00009"}, // doesn't exist
	} {
		if _, err := rollback(&toolCtx{Context: context.Background()}, args); err == nil {
			t.Errorf("%+v: want error", args)
		}
	}
	if f.patches != 1 {
		t.Errorf("patches = %d, want 1", f.patches)
	}
}

func TestChangeToolsOnlyWithTargets(t *testing.T) {
	c := fakeAPI(t)
	if tools, _ := c.ChangeTools(); len(tools) != 0 {
		t.Errorf("no targets: got %d tools, want 0", len(tools))
	}
	c.DeployTargets = []string{"demo-app"}
	tools, err := c.ChangeTools()
	if err != nil || len(tools) != 2 || tools[0].Name() != "deploy" || tools[1].Name() != "rollback" {
		t.Errorf("tools = %v, %v", tools, err)
	}
}
