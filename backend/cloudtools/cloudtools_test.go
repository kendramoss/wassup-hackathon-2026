package cloudtools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/option"
)

const parent = "projects/p/locations/r"

// fakeAPI serves canned Cloud Run and Cloud Build responses keyed by path.
func fakeAPI(t *testing.T) *Client {
	t.Helper()
	responses := map[string]string{
		"/v2/" + parent + "/services": `{"services": [{
			"name": "` + parent + `/services/backend",
			"uri": "https://backend.example",
			"latestReadyRevision": "` + parent + `/services/backend/revisions/backend-00003",
			"terminalCondition": {"type": "Ready", "state": "CONDITION_SUCCEEDED"},
			"trafficStatuses": [
				{"type": "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", "percent": 90},
				{"type": "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", "revision": "backend-00002", "percent": 10, "tag": "old"}
			],
			"updateTime": "2026-10-14T03:00:00Z"
		}]}`,
		"/v2/" + parent + "/services/backend": `{
			"name": "` + parent + `/services/backend",
			"latestReadyRevision": "` + parent + `/services/backend/revisions/backend-00003",
			"trafficStatuses": [
				{"type": "TRAFFIC_TARGET_ALLOCATION_TYPE_LATEST", "percent": 90},
				{"type": "TRAFFIC_TARGET_ALLOCATION_TYPE_REVISION", "revision": "backend-00002", "percent": 10}
			]
		}`,
		"/v2/" + parent + "/services/backend/revisions": `{"revisions": [
			{"name": "` + parent + `/services/backend/revisions/backend-00001", "createTime": "2026-10-12T00:00:00Z",
			 "containers": [{"image": "img@sha256:1"}], "conditions": [{"type": "Ready", "state": "CONDITION_FAILED", "message": "container failed to start"}]},
			{"name": "` + parent + `/services/backend/revisions/backend-00003", "createTime": "2026-10-14T00:00:00Z",
			 "containers": [{"image": "img@sha256:3"}], "conditions": [{"type": "Ready", "state": "CONDITION_SUCCEEDED"}]},
			{"name": "` + parent + `/services/backend/revisions/backend-00002", "createTime": "2026-10-13T00:00:00Z",
			 "containers": [{"image": "img@sha256:2"}], "conditions": [{"type": "Ready", "state": "CONDITION_SUCCEEDED"}]}
		]}`,
		"/v1/" + parent + "/builds": `{"builds": [
			{"id": "aaaaaaaa-1", "status": "SUCCESS", "createTime": "2026-10-14T02:00:00Z",
			 "images": ["us-central1-docker.pkg.dev/p/cloud-run-source-deploy/frontend:latest"]},
			{"id": "bbbbbbbb-2", "status": "FAILURE", "createTime": "2026-10-14T01:00:00Z",
			 "images": ["us-central1-docker.pkg.dev/p/cloud-run-source-deploy/backend:latest"],
			 "failureInfo": {"type": "USER_BUILD_STEP", "detail": "Build step 0 failed"}}
		]}`,
		"/v1/" + parent + "/builds/bbbbbbbb-2": `{"id": "bbbbbbbb-2", "status": "FAILURE",
			"failureInfo": {"type": "USER_BUILD_STEP", "detail": "Build step 0 failed"},
			"logUrl": "https://console.example/logs",
			"steps": [{"id": "Build", "name": "gcr.io/cloud-builders/docker", "status": "FAILURE"}, {"name": "gcr.io/cloud-builders/docker", "status": "QUEUED"}]}`,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		if !ok {
			http.Error(w, `{"error": {"code": 404, "message": "not found"}}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	c, err := New(context.Background(), "p", "r", option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestListServices(t *testing.T) {
	got, err := fakeAPI(t).ListServices(context.Background(), ListServicesArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Services) != 1 {
		t.Fatalf("got %d services, want 1", len(got.Services))
	}
	s := got.Services[0]
	if s.Name != "backend" || s.LatestReadyRevision != "backend-00003" || !s.Ready {
		t.Errorf("service = %+v", s)
	}
	// The "latest" target is reported as the revision it points at.
	want := []traffic{{"backend-00003", 90, ""}, {"backend-00002", 10, "old"}}
	if len(s.Traffic) != 2 || s.Traffic[0] != want[0] || s.Traffic[1] != want[1] {
		t.Errorf("traffic = %+v, want %+v", s.Traffic, want)
	}
}

func TestListRevisions(t *testing.T) {
	c := fakeAPI(t)
	got, err := c.ListRevisions(context.Background(), ListRevisionsArgs{Service: "backend", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Revisions) != 2 {
		t.Fatalf("got %d revisions, want 2 (limit)", len(got.Revisions))
	}
	newest, prev := got.Revisions[0], got.Revisions[1]
	if newest.Name != "backend-00003" || newest.TrafficPercent != 90 || newest.Image != "img@sha256:3" || !newest.Ready {
		t.Errorf("newest = %+v", newest)
	}
	if prev.Name != "backend-00002" || prev.TrafficPercent != 10 {
		t.Errorf("second = %+v", prev)
	}

	all, _ := c.ListRevisions(context.Background(), ListRevisionsArgs{Service: "backend"})
	if failed := all.Revisions[2]; failed.Ready || failed.Problem != "container failed to start" {
		t.Errorf("failed revision = %+v", failed)
	}

	for _, bad := range []string{"", "../secrets", "Backend", "a/b"} {
		if _, err := c.ListRevisions(context.Background(), ListRevisionsArgs{Service: bad}); err == nil {
			t.Errorf("service %q: want error", bad)
		}
	}
}

func TestListBuilds(t *testing.T) {
	c := fakeAPI(t)
	all, err := c.ListBuilds(context.Background(), ListBuildsArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Builds) != 2 {
		t.Fatalf("got %d builds, want 2", len(all.Builds))
	}

	backend, err := c.ListBuilds(context.Background(), ListBuildsArgs{Service: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(backend.Builds) != 1 || backend.Builds[0].ID != "bbbbbbbb-2" {
		t.Fatalf("backend builds = %+v", backend.Builds)
	}
	if f := backend.Builds[0].Failure; !strings.Contains(f, "Build step 0 failed") {
		t.Errorf("failure = %q", f)
	}
}

func TestGetBuild(t *testing.T) {
	c := fakeAPI(t)
	got, err := c.GetBuild(context.Background(), GetBuildArgs{ID: "bbbbbbbb-2"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "FAILURE" || len(got.Steps) != 2 {
		t.Fatalf("build = %+v", got)
	}
	if got.Steps[0] != (buildStep{"Build", "FAILURE"}) || got.Steps[1].Name != "gcr.io/cloud-builders/docker" {
		t.Errorf("steps = %+v", got.Steps)
	}

	if _, err := c.GetBuild(context.Background(), GetBuildArgs{ID: "../x"}); err == nil {
		t.Error("bad id: want error")
	}
	if _, err := c.GetBuild(context.Background(), GetBuildArgs{ID: "cccccccc-3"}); err == nil {
		t.Error("unknown build: want error")
	}
}

func TestTools(t *testing.T) {
	tools, err := fakeAPI(t).Tools()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tl := range tools {
		names = append(names, tl.Name())
	}
	if got := strings.Join(names, ","); got != "list_services,list_revisions,list_builds,get_build" {
		t.Errorf("tools = %s", got)
	}
}
