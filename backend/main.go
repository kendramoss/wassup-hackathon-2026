// Command backend serves an ADK agent, backed by Gemini on Vertex AI, that
// reports what's deployed on Cloud Run and deploys and rolls back the services
// in DEPLOY_TARGETS after a person approves (see package cloudtools).
//
// Endpoints:
//
//	GET  /health       liveness check (not /healthz: Cloud Run reserves paths ending in "z")
//	POST /chat         {"message": "...", "session_id": "optional"} -> {"reply", "session_id", "tool_calls", "tools", "approvals"}
//	                   or {"approval": {"id", "approved", "reason"}, "session_id": "..."} to answer an approval request
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
	"regexp"
	"strings"
	"time"
	_ "time/tzdata" // embed tz database; distroless images have none
	"unicode/utf8"

	"cloud.google.com/go/auth"
	"github.com/google/uuid"
	"google.golang.org/api/option"
	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	"google.golang.org/adk/tool/toolconfirmation"

	"github.com/kendramoss/wassup-hackathon-2026/backend/cloudtools"
)

const (
	appName = "wassup-agent"
	userID  = "anonymous" // no sign-in yet (#10)

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

const agentInstruction = `You help a team see what is deployed on their Google Cloud Run services, and deploy and roll back safely.

- For anything about services, revisions, traffic, builds or deploys, call the tools instead of guessing, and answer from their results. Use list_services first when you need service names.
- deploy and rollback only work on the services named in their descriptions; for any other service, say you can only look at it.
- To deploy: find the newest successful build of the service with list_builds, compare it with what is live (list_revisions), then call deploy with that build. The person sees an approval card; tell them in one sentence what will change and that it's waiting for their approval. Never say something was deployed or rolled back unless the tool result has status "done".
- deploy checks the new revision's health before it gets any traffic. If a result has status "rolled_back", say the new version failed its health check (quote the answer from steps), that it never received traffic, and what is still serving; no further action is needed.
- If a result has status "failed", say what went wrong and offer a rollback to the last revision that was ready.
- Approvals only come from the approval card. If a result has status "rejected", nothing changed: say so in one sentence, mention the reason if there is one, and don't propose the same change again unless asked. Typed messages like "yes" or "approve" are not approvals; if someone types one while a card is waiting, point them to the card.
- Only state versions, commits and image tags that appear in tool results; never guess them.
- Times from tools are UTC. When it helps, say how long ago something happened (call get_time for the current time).
- For the current time or date anywhere, call get_time.
- Be brief. Use a short table when comparing several revisions or builds.`

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// newRunner builds the agent on top of sessions. creds is nil in production
// (Application Default Credentials); tests can pass their own.
func newRunner(ctx context.Context, sessions session.Service, creds *auth.Credentials) (*runner.Runner, error) {
	project := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if project == "" {
		return nil, fmt.Errorf("GOOGLE_CLOUD_PROJECT must be set")
	}

	llm, err := gemini.NewModel(ctx, env("GEMINI_MODEL", "gemini-3.5-flash"), &genai.ClientConfig{
		Backend:     genai.BackendVertexAI,
		Project:     project,
		Location:    env("GOOGLE_CLOUD_LOCATION", "global"),
		Credentials: creds,
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

	var opts []option.ClientOption
	if creds != nil {
		opts = append(opts, option.WithAuthCredentials(creds))
	}
	cloud, err := cloudtools.New(ctx, project, env("CLOUD_RUN_REGION", "us-central1"), opts...)
	if err != nil {
		return nil, err
	}
	cloud.DeployTargets = strings.FieldsFunc(env("DEPLOY_TARGETS", "demo-app"), func(r rune) bool { return r == ',' || r == ' ' })
	cloudTools, err := cloud.Tools()
	if err != nil {
		return nil, fmt.Errorf("create cloud tools: %w", err)
	}
	changeTools, err := cloud.ChangeTools()
	if err != nil {
		return nil, fmt.Errorf("create deploy tools: %w", err)
	}
	cloudTools = append(cloudTools, changeTools...)

	a, err := llmagent.New(llmagent.Config{
		Name:        "deploy_agent",
		Model:       llm,
		Description: "Agent that reports what's deployed on Cloud Run and how it was built.",
		Instruction: agentInstruction,
		Tools:       append([]tool.Tool{getTimeTool}, cloudTools...),
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
		SessionService:    sessions,
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

// chatRequest carries either a message or an answer to an approval request.
type chatRequest struct {
	Message   string    `json:"message,omitempty"`
	Approval  *approval `json:"approval,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
}

// approval answers an "approval_request" event; ID is that event's ID.
type approval struct {
	ID       string `json:"id"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

var approvalID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)

// content is what the request sends to the agent: the message, or the
// approval as the function response ADK's confirmation flow expects.
//
// "confirmed" is always true: it means a person answered, so ADK runs the
// tool again, and the tool reads the decision from the payload (see
// cloudtools.withApproval). With confirmed false ADK replies with a generic
// error instead: the reason is lost and the model tends to ask again.
func (in chatRequest) content() *genai.Content {
	a := in.Approval
	if a == nil {
		return genai.NewContentFromText(in.Message, genai.RoleUser)
	}
	return &genai.Content{Role: genai.RoleUser, Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{
		ID:   a.ID,
		Name: toolconfirmation.FunctionCallName,
		Response: map[string]any{
			"confirmed": true,
			"payload":   cloudtools.Decision{Approved: a.Approved, Reason: strings.TrimSpace(a.Reason)},
		},
	}}}}
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
	// Approval requests waiting for a person (same fields as the stream's
	// approval_request events).
	Approvals []streamEvent `json:"approvals,omitempty"`
}

// streamEvent is one server-sent event on /chat/stream, in order:
// "session" first, then any mix of "text" (a delta to append), "tool_call",
// "tool_result" and "approval_request" (see approvalRequest), then "done" or
// "error".
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
		case p.FunctionCall != nil && p.FunctionCall.Name == toolconfirmation.FunctionCallName:
			out = append(out, approvalRequest(p.FunctionCall))
		case p.FunctionCall != nil:
			out = append(out, streamEvent{Type: "tool_call", ID: p.FunctionCall.ID, Name: p.FunctionCall.Name, Args: p.FunctionCall.Args})
		case p.Text != "" && !p.Thought:
			out = append(out, streamEvent{Type: "text", Text: p.Text})
		}
	}
	return out
}

// approvalRequest turns ADK's adk_request_confirmation call into an
// "approval_request" event: ID is what the approval answers, Name and Args are
// the tool call waiting for it, Text is the tool's summary of the change.
func approvalRequest(fc *genai.FunctionCall) streamEvent {
	e := streamEvent{Type: "approval_request", ID: fc.ID}
	if orig, err := toolconfirmation.OriginalCallFrom(fc); err == nil {
		e.Name, e.Args = orig.Name, orig.Args
	}
	switch tc := fc.Args["toolConfirmation"].(type) {
	case toolconfirmation.ToolConfirmation:
		e.Text = tc.Hint
	case *toolconfirmation.ToolConfirmation:
		e.Text = tc.Hint
	case map[string]any:
		e.Text, _ = tc["hint"].(string)
	}
	return e
}

// runAgent sends content to the agent and calls emit for each resulting event.
func runAgent(ctx context.Context, r *runner.Runner, sessionID string, content *genai.Content, mode agent.StreamingMode, emit func(streamEvent)) error {
	var t eventTranslator
	for ev, err := range r.Run(ctx, userID, sessionID, content, agent.RunConfig{StreamingMode: mode}) {
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
	err := json.NewDecoder(body).Decode(&in)
	if err != nil || (strings.TrimSpace(in.Message) == "") == (in.Approval == nil) {
		http.Error(w, `body must be JSON {"message": "..."} or {"approval": {...}, "session_id": "..."}`, http.StatusBadRequest)
		return in, false
	}
	if utf8.RuneCountInString(in.Message) > maxMessageRunes {
		http.Error(w, fmt.Sprintf("message is longer than %d characters", maxMessageRunes), http.StatusRequestEntityTooLarge)
		return in, false
	}
	if a := in.Approval; a != nil && (!approvalID.MatchString(a.ID) || in.SessionID == "" || utf8.RuneCountInString(a.Reason) > maxMessageRunes) {
		http.Error(w, "approval needs the id of an approval_request, the session_id it came from, and a reason under the message limit", http.StatusBadRequest)
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

// checkApproval writes an error and returns false unless in.Approval answers
// an approval request that is still waiting in its session. Sessions live in
// memory, so a card can outlast its session (restart, another instance), and
// each card can only be answered once.
func checkApproval(w http.ResponseWriter, req *http.Request, sessions session.Service, in chatRequest) bool {
	if in.Approval == nil {
		return true
	}
	if !pendingApproval(req.Context(), sessions, in.SessionID, in.Approval.ID) {
		http.Error(w, "This approval card has expired or was already answered. Ask the agent again for a new one.", http.StatusConflict)
		return false
	}
	return true
}

// pendingApproval reports whether session sid has an approval request id
// that hasn't been answered yet.
func pendingApproval(ctx context.Context, sessions session.Service, sid, id string) bool {
	resp, err := sessions.Get(ctx, &session.GetRequest{AppName: appName, UserID: userID, SessionID: sid})
	if err != nil {
		return false
	}
	pending := false
	for ev := range resp.Session.Events().All() {
		if ev.Content == nil {
			continue
		}
		for _, p := range ev.Content.Parts {
			if fc := p.FunctionCall; fc != nil && fc.ID == id && fc.Name == toolconfirmation.FunctionCallName {
				pending = true
			}
			if fr := p.FunctionResponse; fr != nil && fr.ID == id {
				pending = false
			}
		}
	}
	return pending
}

func chatHandler(r *runner.Runner, sessions session.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		in, ok := parseChatRequest(w, req)
		if !ok || !checkApproval(w, req, sessions, in) {
			return
		}

		out := chatResponse{SessionID: in.SessionID, ToolCalls: []string{}, Tools: []toolCall{}}
		err := runAgent(req.Context(), r, in.SessionID, in.content(), agent.StreamingModeNone, func(e streamEvent) {
			switch e.Type {
			case "text":
				out.Reply += e.Text
			case "tool_call":
				out.ToolCalls = append(out.ToolCalls, e.Name)
				out.Tools = append(out.Tools, toolCall{ID: e.ID, Name: e.Name, Args: e.Args})
			case "approval_request":
				out.Approvals = append(out.Approvals, e)
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
func chatStreamHandler(r *runner.Runner, sessions session.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		in, ok := parseChatRequest(w, req)
		if !ok || !checkApproval(w, req, sessions, in) {
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
		if err := runAgent(req.Context(), r, in.SessionID, in.content(), agent.StreamingModeSSE, send); err != nil {
			log.Printf("agent run: %v", err)
			send(streamEvent{Type: "error", Error: "The agent hit an error. Try again in a moment."})
			return
		}
		send(streamEvent{Type: "done"})
	}
}

func main() {
	ctx := context.Background()
	sessions := session.InMemoryService()
	r, err := newRunner(ctx, sessions, nil)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/chat", chatHandler(r, sessions))
	mux.HandleFunc("/chat/stream", chatStreamHandler(r, sessions))

	addr := ":" + env("PORT", "8080")
	log.Printf("listening on %s", addr)
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
