// Package llm is the seam between Origin Proof's business logic and model
// providers. Engine, router, planner and verifier depend only on the Model
// interface; implementations are the LiteLLM proxy (local development only),
// Vertex AI through google.golang.org/genai (deployment), and Fake (tests).
//
// Every call is deterministic by contract: temperature 0 and, when a JSON
// answer is expected, a response JSON schema.
package llm
