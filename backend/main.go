// Command backend serves a hello-world ADK agent backed by Gemini on Vertex AI.
//
// Endpoints:
//
//	GET  /health       liveness check (not /healthz: Cloud Run reserves paths ending in "z")
//	POST /chat         {"message": "...", "session_id": "optional"} -> {"reply", "session_id", "tool_calls", "tools"}
//	POST /chat/stream  same request; reply as server-sent events (see streamEvent)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
	_ "time/tzdata" // embed tz database; distroless images have none
	"unicode/utf8"

	"github.com/google/uuid"
	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
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

	llm, err := gemini.NewModel(ctx, env("GEMINI_MODEL", "gemini-3.5-flash"), &genai.ClientConfig{
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
		Model:       llm,
		Description: "Hello-world agent that can tell the time.",
		Instruction: "You are a friendly assistant. When the user asks about the current time or date, always call the get_time tool rather than guessing.",
		Tools:       []tool.Tool{getTimeTool},
		BeforeModelCallbacks: []llmagent.BeforeModelCallback{
			func(_ agent.CallbackContext, req *model.LLMRequest) (*model.LLMResponse, error) {
				req.Contents = dropEmptyParts(req.Contents)
				return nil, nil
			},
		},
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

// dropEmptyParts returns contents without parts that carry no data. In
// streaming mode ADK v1.8.0 stores Gemini 3's final chunk, an empty text part
// with only a thought signature, in the session history; Vertex rejects the
// next turn's request with 400 INVALID_ARGUMENT if it's sent back. Builds new
// slices so stored session events aren't modified.
func dropEmptyParts(contents []*genai.Content) []*genai.Content {
	out := make([]*genai.Content, 0, len(contents))
	for _, c := range contents {
		if c == nil {
			continue
		}
		parts := make([]*genai.Part, 0, len(c.Parts))
		for _, p := range c.Parts {
			if p == nil {
				continue
			}
			data := *p
			data.Thought, data.ThoughtSignature = false, nil
			if !reflect.ValueOf(data).IsZero() {
				parts = append(parts, p)
			}
		}
		if len(parts) > 0 {
			cc := *c
			cc.Parts = parts
			out = append(out, &cc)
		}
	}
	return out
}

type chatRequest struct {
	Message   string `json:"message"`
	SessionID string `json:"session_id,omitempty"`
}

type toolCall struct {
	ID     string         `json:"id,omitempty"`
	Name   string         `json:"name"`
	Args   map[string]any `json:"args,omitempty"`
	Result map[string]any `json:"result,omitempty"`
}

type chatResponse struct {
	Reply     string     `json:"reply"`
	SessionID string     `json:"session_id"`
	ToolCalls []string   `json:"tool_calls"` // names only; kept for scripts and the smoke test
	Tools     []toolCall `json:"tools"`
}

// streamEvent is one server-sent event on /chat/stream, in order:
// "session" first, then any mix of "text" (a delta to append), "tool_call"
// and "tool_result", then "done" or "error".
type streamEvent struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	Result    map[string]any `json:"result,omitempty"`
	Error     string         `json:"error,omitempty"`
}

// eventTranslator turns ADK events into stream events. In streaming mode a
// model response arrives as partial events and is then repeated in full (text
// and tool calls) in a final non-partial event; that repeat is dropped.
type eventTranslator struct {
	sawPartial bool
}

func (t *eventTranslator) translate(ev *session.Event) []streamEvent {
	repeat := !ev.Partial && t.sawPartial
	t.sawPartial = ev.Partial
	if ev.Content == nil {
		return nil
	}
	var out []streamEvent
	for _, p := range ev.Content.Parts {
		switch {
		case p.FunctionResponse != nil:
			out = append(out, streamEvent{Type: "tool_result", ID: p.FunctionResponse.ID, Name: p.FunctionResponse.Name, Result: p.FunctionResponse.Response})
		case repeat:
		case p.FunctionCall != nil:
			out = append(out, streamEvent{Type: "tool_call", ID: p.FunctionCall.ID, Name: p.FunctionCall.Name, Args: p.FunctionCall.Args})
		case p.Text != "" && !p.Thought:
			out = append(out, streamEvent{Type: "text", Text: p.Text})
		}
	}
	return out
}

// runAgent sends msg to the agent and calls emit for each resulting event.
func runAgent(ctx context.Context, r *runner.Runner, sessionID, msg string, mode agent.StreamingMode, emit func(streamEvent)) error {
	var t eventTranslator
	content := genai.NewContentFromText(msg, genai.RoleUser)
	for ev, err := range r.Run(ctx, "anonymous", sessionID, content, agent.RunConfig{StreamingMode: mode}) {
		if err != nil {
			return err
		}
		for _, e := range t.translate(ev) {
			emit(e)
		}
	}
	return nil
}

// parseChatRequest validates a /chat or /chat/stream request, writing the
// error response itself when it returns false.
func parseChatRequest(w http.ResponseWriter, req *http.Request) (chatRequest, bool) {
	var in chatRequest
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return in, false
	}
	body := http.MaxBytesReader(w, req.Body, maxBodyBytes)
	if err := json.NewDecoder(body).Decode(&in); err != nil || strings.TrimSpace(in.Message) == "" {
		http.Error(w, `body must be JSON {"message": "..."}`, http.StatusBadRequest)
		return in, false
	}
	if utf8.RuneCountInString(in.Message) > maxMessageRunes {
		http.Error(w, fmt.Sprintf("message is longer than %d characters", maxMessageRunes), http.StatusRequestEntityTooLarge)
		return in, false
	}
	// Session IDs are issued by this server; reject anything that isn't one.
	if in.SessionID == "" {
		in.SessionID = uuid.NewString()
	} else if _, err := uuid.Parse(in.SessionID); err != nil {
		http.Error(w, "session_id must be a UUID returned by a previous /chat call", http.StatusBadRequest)
		return in, false
	}
	return in, true
}

func chatHandler(r *runner.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		in, ok := parseChatRequest(w, req)
		if !ok {
			return
		}

		out := chatResponse{SessionID: in.SessionID, ToolCalls: []string{}, Tools: []toolCall{}}
		err := runAgent(req.Context(), r, in.SessionID, in.Message, agent.StreamingModeNone, func(e streamEvent) {
			switch e.Type {
			case "text":
				out.Reply += e.Text
			case "tool_call":
				out.ToolCalls = append(out.ToolCalls, e.Name)
				out.Tools = append(out.Tools, toolCall{ID: e.ID, Name: e.Name, Args: e.Args})
			case "tool_result":
				for i := range out.Tools {
					if out.Tools[i].ID == e.ID && out.Tools[i].Result == nil {
						out.Tools[i].Result = e.Result
						break
					}
				}
			}
		})
		if err != nil {
			log.Printf("agent run: %v", err)
			http.Error(w, "agent error", http.StatusBadGateway)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	}
}

// chatStreamHandler is /chat as server-sent events, so replies show up as
// they are generated.
func chatStreamHandler(r *runner.Runner) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		in, ok := parseChatRequest(w, req)
		if !ok {
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		// no-transform stops proxies (and Next's compression) from buffering.
		w.Header().Set("Cache-Control", "no-cache, no-transform")
		w.Header().Set("X-Accel-Buffering", "no")
		rc := http.NewResponseController(w)
		send := func(e streamEvent) {
			b, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", b)
			rc.Flush()
		}

		send(streamEvent{Type: "session", SessionID: in.SessionID})
		if err := runAgent(req.Context(), r, in.SessionID, in.Message, agent.StreamingModeSSE, send); err != nil {
			log.Printf("agent run: %v", err)
			send(streamEvent{Type: "error", Error: "The agent hit an error. Try again in a moment."})
			return
		}
		send(streamEvent{Type: "done"})
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
	mux.HandleFunc("/chat/stream", chatStreamHandler(r))

	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
