// Command backend serves a hello-world ADK agent backed by Gemini on Vertex AI.
//
// Endpoints:
//
//	GET  /health   liveness check (not /healthz: Cloud Run reserves paths ending in "z")
//	POST /chat     {"message": "...", "session_id": "optional"} -> {"reply", "session_id", "tool_calls"}
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	_ "time/tzdata" // embed tz database; distroless images have none
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	appName = "wassup-agent"

	// Limits on /chat input, so one request can't burn a large prompt.
	maxBodyBytes    = 16 << 10
	maxMessageRunes = 2000
)

type getTimeArgs struct {
	Timezone string `json:"timezone,omitempty" jsonschema:"IANA timezone name, e.g. Asia/Singapore. Defaults to UTC."`
}

type getTimeResult struct {
	Timezone string `json:"timezone"`
	Time     string `json:"time"`
}

func getTime(_ agent.ToolContext, args getTimeArgs) (getTimeResult, error) {
	tz := args.Timezone
	if tz == "" {
		tz = "UTC"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return getTimeResult{}, fmt.Errorf("unknown timezone %q", tz)
	}
	return getTimeResult{Timezone: tz, Time: time.Now().In(loc).Format(time.RFC3339)}, nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newRunner(ctx context.Context) (*runner.Runner, error) {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT must be set")
	}

	model, err := gemini.NewModel(ctx, env("GEMINI_MODEL", "gemini-3.8-flash"), &genai.ClientConfig{
		Backend:  genai.BackendVertexAI,
		Project:  project,
		Location: env("GOOGLE_CLOUD_LOCATION", "global"),
	})
	if err != nil {
		return nil, fmt.Errorf("create model: %w", err)
	}

	getTimeTool, err := functiontool.New(functiontool.Config{
		Name:        "get_time",
		Description: "Returns the current time in the given IANA timezone.",
	}, getTime)
	if err != nil {
		return nil, fmt.Errorf("create get_time tool: %w", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "hello_agent",
		Model:       model,
		Description: "Hello-world agent that can tell the time.",
		Instruction: "You are a friendly assistant. When the user asks about the current time or date, always call the get_time tool rather than guessing.",
		Tools:       []tool.Tool{getTimeTool},
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	return runner.New(runner.Config{
		AppName:           appName,
		Agent:             a,
		SessionService:    session.InMemoryService(),
		AutoCreateSession: true,
	})
}

type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

type chatResponse struct {
	Reply     string   `json:"reply"`
	SessionID string   `json:"session_id"`
	ToolCalls []string `json:"tool_calls"`
}

func chatHandler(r *runner.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var in chatRequest
		body := http.MaxBytesReader(w, req.Body, maxBodyBytes)
		if err := json.NewDecoder(body).Decode(&in); err != nil || strings.TrimSpace(in.Message) == "" {
			http.Error(w, `body must be JSON {"message": "..."}`, http.StatusBadRequest)
			return
		}
		if utf8.RuneCountInString(in.Message) > maxMessageRunes {
			http.Error(w, fmt.Sprintf("message is longer than %d characters", maxMessageRunes), http.StatusRequestEntityTooLarge)
			return
		}
		// Session IDs are issued by this server; reject anything that isn't one.
		if in.SessionID == "" {
			in.SessionID = uuid.NewString()
		} else if _, err := uuid.Parse(in.SessionID); err != nil {
			http.Error(w, "session_id must be a UUID returned by a previous /chat call", http.StatusBadRequest)
			return
		}

		out := chatResponse{SessionID: in.SessionID, ToolCalls: []string{}}
		msg := genai.NewContentFromText(in.Message, genai.RoleUser)
		for ev, err := range r.Run(req.Context(), "anonymous", in.SessionID, msg, agent.RunConfig{}) {
			if err != nil {
				log.Printf("agent run: %v", err)
				http.Error(w, "agent error", http.StatusBadGateway)
				return
			}
			if ev.Content == nil {
				continue
			}
			for _, p := range ev.Content.Parts {
				if p.FunctionCall != nil {
					out.ToolCalls = append(out.ToolCalls, p.FunctionCall.Name)
				}
			}
			if ev.IsFinalResponse() {
				for _, p := range ev.Content.Parts {
					out.Reply += p.Text
				}
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

func main() {
	ctx := context.Background()
	r, err := newRunner(ctx)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/chat", chatHandler(r))

	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
