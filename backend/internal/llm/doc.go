// Package llm is the seam between Unde's business logic and model
// providers. Engine, router, planner and verifier depend only on the Model
// interface; implementations are Vertex AI through google.golang.org/genai
// (development, evals and deployment) and Fake (tests). The LiteLLM proxy
// used before the team had Vertex access was removed from the server (D49);
// LiteLLM (litellm.go) is back for offline eval runs only (cmd/eval, D151),
// and the server's config still refuses it.
//
// Every call is deterministic by contract: temperature 0 and, when a JSON
// answer is expected, a response JSON schema.
package llm
