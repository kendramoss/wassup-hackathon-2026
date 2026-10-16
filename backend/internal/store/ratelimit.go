package store

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	vkit "cloud.google.com/go/firestore/apiv1"
	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/ratelimit"
)

// RateLimitCollection holds the shared rate limiter's fixed-window counters
// (D70): one document per client, class and minute, and one per minute for
// the global cap. Each has n (the count) and expires_at (the window's end);
// a TTL policy on expires_at lets Firestore delete ended windows.
const RateLimitCollection = "rate_limits"

// FirestoreCounters implements ratelimit.Store. It uses the low-level
// Firestore API because only a raw Commit returns the incremented values:
// one round trip increments every counter a request touches and reads back
// the new totals. The writes of one Commit are applied atomically.
type FirestoreCounters struct {
	client *vkit.Client
	db     string // projects/<p>/databases/<d>
}

// NewFirestoreCounters connects to database (empty means "(default)") in
// project with Application Default Credentials, or to the emulator when
// FIRESTORE_EMULATOR_HOST is set (as the Firestore client does).
func NewFirestoreCounters(ctx context.Context, project, database string) (*FirestoreCounters, error) {
	var opts []option.ClientOption
	if addr := os.Getenv("FIRESTORE_EMULATOR_HOST"); addr != "" {
		conn, err := grpc.NewClient("passthrough:///"+addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithPerRPCCredentials(emulatorOwner{}))
		if err != nil {
			return nil, fmt.Errorf("store: rate counters emulator %q: %w", addr, err)
		}
		opts = append(opts, option.WithGRPCConn(conn))
	}
	c, err := vkit.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("store: rate counters client (project %q, database %q): %w", project, database, err)
	}
	return NewFirestoreCountersFromClient(c, project, database), nil
}

// NewFirestoreCountersFromClient wraps an existing low-level client (tests
// use a fake server). Automatic retries are turned off: a retried Commit
// could count a request twice, and the limiter falls back to its local
// buckets on an error anyway.
func NewFirestoreCountersFromClient(c *vkit.Client, project, database string) *FirestoreCounters {
	if database == "" {
		database = firestore.DefaultDatabaseID
	}
	c.CallOptions.Commit = nil
	return &FirestoreCounters{client: c, db: fmt.Sprintf("projects/%s/databases/%s", project, database)}
}

// Close releases the client.
func (f *FirestoreCounters) Close() error { return f.client.Close() }

// Add implements ratelimit.Store.
func (f *FirestoreCounters) Add(ctx context.Context, cs []ratelimit.Counter, expires time.Time) ([]int64, error) {
	exp := &pb.Value{ValueType: &pb.Value_TimestampValue{TimestampValue: timestamppb.New(expires)}}
	writes := make([]*pb.Write, len(cs))
	for i, c := range cs {
		if c.ID == "" || strings.Contains(c.ID, "/") || strings.HasPrefix(c.ID, "__") {
			return nil, fmt.Errorf("store: invalid rate counter id %q", c.ID)
		}
		writes[i] = &pb.Write{
			Operation: &pb.Write_Update{Update: &pb.Document{
				Name:   f.db + "/documents/" + RateLimitCollection + "/" + c.ID,
				Fields: map[string]*pb.Value{"expires_at": exp},
			}},
			UpdateMask: &pb.DocumentMask{FieldPaths: []string{"expires_at"}}, // n is left to the transform
			UpdateTransforms: []*pb.DocumentTransform_FieldTransform{{
				FieldPath:     "n",
				TransformType: &pb.DocumentTransform_FieldTransform_Increment{Increment: &pb.Value{ValueType: &pb.Value_IntegerValue{IntegerValue: c.N}}},
			}},
		}
	}
	ctx = metadata.AppendToOutgoingContext(ctx, "google-cloud-resource-prefix", f.db)
	resp, err := f.client.Commit(ctx, &pb.CommitRequest{Database: f.db, Writes: writes})
	if err != nil {
		return nil, fmt.Errorf("store: rate counters: %w", err)
	}
	if len(resp.GetWriteResults()) != len(cs) {
		return nil, fmt.Errorf("store: rate counters: %d results for %d writes", len(resp.GetWriteResults()), len(cs))
	}
	out := make([]int64, len(cs))
	for i, wr := range resp.GetWriteResults() {
		tr := wr.GetTransformResults()
		if len(tr) != 1 {
			return nil, fmt.Errorf("store: rate counter %s: %d transform results", cs[i].ID, len(tr))
		}
		switch v := tr[0].GetValueType().(type) {
		case *pb.Value_IntegerValue:
			out[i] = v.IntegerValue
		case *pb.Value_DoubleValue: // only if something wrote n as a double
			out[i] = int64(v.DoubleValue)
		default:
			return nil, fmt.Errorf("store: rate counter %s: unexpected value %T", cs[i].ID, v)
		}
	}
	return out, nil
}

// emulatorOwner is the credential the Firestore emulator accepts as admin.
type emulatorOwner struct{}

func (emulatorOwner) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer owner"}, nil
}

func (emulatorOwner) RequireTransportSecurity() bool { return false }
