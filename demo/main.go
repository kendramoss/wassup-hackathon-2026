// Command demo-app is the sample service the agent deploys and rolls back.
// GitHub Actions only builds its images; deploying them is the agent's job.
//
//	GET /        a page showing which version is live
//	GET /health  "ok", or 500 when healthy is false
package main

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"os"
	"time"
)

// version is the commit this image was built from (set with -ldflags -X by
// the Dockerfile, from the VERSION file the workflow writes).
var version = "dev"

// healthy: set to false and push to get a build that fails its health
// check, to show the agent catching a bad deploy.
const healthy = true

const page = `<!doctype html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Demo app</title>
<style>
  :root { color-scheme: light dark; font-family: system-ui, sans-serif; }
  body { margin: 0; min-height: 100dvh; display: grid; place-items: center; padding: 16px; }
  main { max-width: 32rem; }
  h1 { font-size: 1.5rem; margin: 0 0 .5rem; }
  p { margin: .25rem 0; color: GrayText; }
  code { font-size: 1.25rem; }
</style>
<main>
  <h1>Demo app</h1>
  <p>Version <code>%s</code>, started %s</p>
  <p>The agent deploys this service and rolls it back.</p>
</main>
`

func newMux(started string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, page, html.EscapeString(version), started)
	})
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		if !healthy {
			http.Error(w, "unhealthy", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	})
	return mux
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("demo-app %s listening on :%s", version, port)
	mux := newMux(time.Now().UTC().Format(time.RFC3339))
	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
