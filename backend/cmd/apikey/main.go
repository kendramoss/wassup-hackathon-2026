// Command apikey issues, lists and disables broker API keys (D69) straight
// in Firestore, the store the server reads them from. The backend is private
// on Cloud Run and the public proxy does not forward the admin token, so this
// is how an admin with project access issues a key (D95).
//
//	go run ./cmd/apikey issue -name "Acme Brokerage" -quota 200
//	go run ./cmd/apikey list
//	go run ./cmd/apikey disable <id>
//
// Needs Application Default Credentials with Firestore access
// (gcloud auth application-default login) and GOOGLE_CLOUD_PROJECT;
// FIRESTORE_DATABASE picks a non-default database. The secret is printed
// once and never stored: only its hash is.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/apikey"
	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		fail("set GOOGLE_CLOUD_PROJECT (wassup-hackathon-2026)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fs, err := store.NewFirestore(ctx, project, os.Getenv("FIRESTORE_DATABASE"))
	if err != nil {
		fail("firestore: %v", err)
	}
	keys := apikey.New(fs.APIKeys())
	switch os.Args[1] {
	case "issue":
		fl := flag.NewFlagSet("issue", flag.ExitOnError)
		name := fl.String("name", "", "who the key is for (shown in the list)")
		quota := fl.Int("quota", 200, "requests per UTC day")
		_ = fl.Parse(os.Args[2:])
		secret, k, err := keys.Issue(ctx, *name, *quota)
		if err != nil {
			fail("issue: %v", err)
		}
		fmt.Fprintf(os.Stderr, "issued %s (%q, %d requests a day). The key is shown once; send it to the broker privately:\n", k.ID, k.Name, k.DailyQuota)
		fmt.Println(secret)
	case "list":
		ks, err := keys.List(ctx)
		if err != nil {
			fail("list: %v", err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tDAILY QUOTA\tCREATED\tDISABLED")
		for _, k := range ks {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%t\n", k.ID, k.Name, k.DailyQuota, k.CreatedAt.Format(time.DateOnly), k.Disabled)
		}
		_ = w.Flush()
	case "disable":
		if len(os.Args) < 3 {
			usage()
		}
		if err := keys.Disable(ctx, os.Args[2]); err != nil {
			fail("disable: %v", err)
		}
		fmt.Fprintf(os.Stderr, "disabled %s\n", os.Args[2])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: apikey issue -name NAME [-quota N] | list | disable ID")
	os.Exit(2)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "apikey: "+format+"\n", a...)
	os.Exit(1)
}
