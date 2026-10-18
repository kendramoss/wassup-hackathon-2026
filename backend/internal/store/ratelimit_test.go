package store

import (
	"context"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	vkit "cloud.google.com/go/firestore/apiv1"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
)

// fakeFirestore answers Commit the way Firestore does for an upsert with an
// increment transform: the field is added to (created at 0) and the new
// value comes back as the write's transform result.
type fakeFirestore struct {
	pb.UnimplementedFirestoreServer
	mu      sync.Mutex
	n       map[string]int64
	expires map[string]time.Time
	commits []*pb.CommitRequest
	fail    error
}

func (f *fakeFirestore) Commit(_ context.Context, req *pb.CommitRequest) (*pb.CommitResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = append(f.commits, req)
	if f.fail != nil {
		return nil, f.fail
	}
	var out []*pb.WriteResult
	for _, w := range req.GetWrites() {
		doc := w.GetUpdate()
		if doc == nil || len(w.GetUpdateTransforms()) != 1 || w.GetUpdateTransforms()[0].GetFieldPath() != "n" {
			return nil, status.Error(codes.InvalidArgument, "unexpected write")
		}
		if got := w.GetUpdateMask().GetFieldPaths(); len(got) != 1 || got[0] != "expires_at" {
			return nil, status.Errorf(codes.InvalidArgument, "mask %v would overwrite n", got)
		}
		f.expires[doc.GetName()] = doc.GetFields()["expires_at"].GetTimestampValue().AsTime()
		f.n[doc.GetName()] += w.GetUpdateTransforms()[0].GetIncrement().GetIntegerValue()
		out = append(out, &pb.WriteResult{TransformResults: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: f.n[doc.GetName()]}}}})
	}
	return &pb.CommitResponse{WriteResults: out}, nil
}

func newFakeCounters(t *testing.T) (*FirestoreCounters, *fakeFirestore) {
	t.Helper()
	fake := &fakeFirestore{n: map[string]int64{}, expires: map[string]time.Time{}}
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	pb.RegisterFirestoreServer(srv, fake)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	c, err := vkit.NewClient(context.Background(), option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	fc := NewFirestoreCountersFromClient(c, "demo-unde", "")
	t.Cleanup(func() { _ = fc.Close() })
	return fc, fake
}

func TestFirestoreCountersOneCommitPerAdd(t *testing.T) {
	fc, fake := newFakeCounters(t)
	ctx := context.Background()
	end := time.Date(2026, 10, 17, 12, 1, 0, 0, time.UTC)
	cs := []ratelimit.Counter{{ID: "abc-light-1", N: 1}, {ID: "global-1", N: 5}}
	if got, err := fc.Add(ctx, cs, end); err != nil || got[0] != 1 || got[1] != 5 {
		t.Fatalf("first Add = %v %v", got, err)
	}
	got, err := fc.Add(ctx, cs, end)
	if err != nil || got[0] != 2 || got[1] != 10 {
		t.Fatalf("second Add = %v %v", got, err)
	}
	if len(fake.commits) != 2 {
		t.Errorf("%d commits for 2 Adds, want one round trip each", len(fake.commits))
	}
	name := "projects/demo-unde/databases/(default)/documents/rate_limits/global-1"
	if req := fake.commits[0]; req.GetDatabase() != "projects/demo-unde/databases/(default)" || req.GetWrites()[1].GetUpdate().GetName() != name {
		t.Errorf("commit addressed %q / %q", req.GetDatabase(), req.GetWrites()[1].GetUpdate().GetName())
	}
	if !fake.expires[name].Equal(end) {
		t.Errorf("expires_at = %v, want %v (the TTL field)", fake.expires[name], end)
	}
	if _, err := fc.Add(ctx, []ratelimit.Counter{{ID: "a/b", N: 1}}, end); err == nil {
		t.Error("a counter ID with a slash was accepted")
	}
}

func TestFirestoreCountersErrorIsNotRetried(t *testing.T) {
	fc, fake := newFakeCounters(t)
	fake.fail = status.Error(codes.Unavailable, "down")
	_, err := fc.Add(context.Background(), []ratelimit.Counter{{ID: "x-light-1", N: 1}}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "down") {
		t.Fatalf("Add on a failing store = %v", err)
	}
	if len(fake.commits) != 1 {
		t.Errorf("%d commits, want 1: a retried increment could count a request twice", len(fake.commits))
	}
}

// TestFirestoreCountersEmulator runs against the Firestore emulator:
//
//	gcloud emulators firestore start --host-port=localhost:8681
//	FIRESTORE_EMULATOR_HOST=localhost:8681 go test ./internal/store
func TestFirestoreCountersEmulator(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	ctx := context.Background()
	a, err := NewFirestoreCounters(ctx, "demo-unde", "")
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := NewFirestoreCounters(ctx, "demo-unde", "") // another instance
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	id := "emu-light-" + time.Now().Format("150405.000000")
	end := time.Now().Add(time.Minute)
	if got, err := a.Add(ctx, []ratelimit.Counter{{ID: id, N: 2}}, end); err != nil || got[0] != 2 {
		t.Fatalf("a.Add = %v %v", got, err)
	}
	if got, err := b.Add(ctx, []ratelimit.Counter{{ID: id, N: 1}}, end); err != nil || got[0] != 3 {
		t.Fatalf("b.Add = %v %v, want the shared total 3", got, err)
	}
	if got, err := a.Add(ctx, []ratelimit.Counter{{ID: id, N: -1}}, end); err != nil || got[0] != 2 {
		t.Fatalf("undo = %v %v", got, err)
	}
}
