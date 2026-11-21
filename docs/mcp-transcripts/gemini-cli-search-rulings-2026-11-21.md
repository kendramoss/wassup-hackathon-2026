# Gemini CLI calling Unde over MCP (run 21 Nov 2026)

Status: run end to end once, on 21 Nov 2026, against the production endpoint `https://undelabs.dev/mcp`. Gemini CLI 0.69.0 in headless mode, model Gemini 3.8 Flash. The CLI reached the model through an OpenAI-compatible LiteLLM gateway to Gemini, which also serves Gemini's own `generateContent` API; `GOOGLE_GEMINI_BASE_URL` points the CLI at it and the gateway key goes in `GEMINI_API_KEY`. No Google API key was used. Only the read-only tools `search_rulings` and `get_ruling` were offered, so no case was created and no model call ran on the Unde side. Model cost: about $0.01 (18,704 tokens on Gemini 3.8 Flash, estimated from the token count; the CLI does not print the gateway's cost).

## What we ran

A scratch home directory kept this run away from any other Gemini CLI settings.

```bash
npm install @google/gemini-cli            # 0.69.0
export HOME=$PWD/home                       # scratch settings for this run
export GEMINI_API_KEY="$LITELLM_API_KEY"    # the gateway key (not in this repository)
export GOOGLE_GEMINI_BASE_URL="$LITELLM_BASE_URL"
mkdir -p home/.gemini work && cd work
# home/.gemini/settings.json and trustedFolders.json as below
npx gemini mcp list
npx gemini -m gemini-3.8-flash --output-format stream-json -p "Use the unde search_rulings tool with the query 'heat gun motor assembled Thailand' and list the first three ruling numbers with their held origin. Do not use any other tool."
```

`home/.gemini/settings.json`:

```json
{
  "security": {
    "auth": {
      "selectedType": "gemini-api-key"
    }
  },
  "privacy": {
    "usageStatisticsEnabled": false
  },
  "telemetry": {
    "enabled": false
  },
  "mcpServers": {
    "unde": {
      "httpUrl": "https://undelabs.dev/mcp",
      "includeTools": [
        "search_rulings",
        "get_ruling"
      ],
      "trust": true
    }
  }
}
```

`home/.gemini/trustedFolders.json` marks the `work` folder `TRUST_FOLDER`. Two things the documented one-liner (`gemini mcp add --transport http unde https://undelabs.dev/mcp`) does not mention:

- Gemini CLI disables every MCP server in a folder it does not trust. `gemini mcp list` first printed "MCP servers are configured but disabled because this folder is untrusted" until the folder was trusted.
- In headless mode (`-p`) a tool call needs approval. `"trust": true` on the server skips the prompt; we limited it to the two read-only tools with `includeTools`.

With a Google API key, leave out `GOOGLE_GEMINI_BASE_URL` and set `GEMINI_API_KEY` to that key.

## Transcript

`gemini mcp list`:

```text
Configured MCP servers:

✓ unde: https://undelabs.dev/mcp (http) - Connected
```

The `stream-json` output. Redacted: the session id is removed, and the streamed answer chunks are joined into one message. Nothing else is changed.

```json
{"type": "init", "timestamp": "2026-11-21T14:54:36.064Z", "model": "gemini-3.8-flash"}
{"type": "message", "timestamp": "2026-11-21T14:54:36.066Z", "role": "user", "content": "Use the unde search_rulings tool with the query 'heat gun motor assembled Thailand' and list the first three ruling numbers with their held origin. Do not use any other tool."}
{"type": "tool_use", "timestamp": "2026-11-21T14:54:39.974Z", "tool_name": "mcp_unde_search_rulings", "tool_id": "mcp_unde_search_rulings__call_11765776988240696635", "parameters": {"limit": 3, "query": "heat gun motor assembled Thailand"}}
{"type": "tool_result", "timestamp": "2026-11-21T14:54:40.480Z", "tool_id": "mcp_unde_search_rulings__call_11765776988240696635", "status": "success", "output": "3 rulings found.\nN357771 (2026-01-29): The country of origin of a heat gun. Held origin: Thailand.\nN359646 (2026-03-26): The country of origin of a caulking gun. Held origin: Vietnam.\nN347556 (2025-05-06): The country of origin of a manual grease gun. Held origin: Vietnam.\n\nNot a ruling. Confirm with a licensed customs broker or a CBP binding ruling."}
{"type": "message", "timestamp": "2026-11-21T14:54:43.752Z", "role": "assistant", "content": "Here are the first three ruling numbers and their held origins:\n\n1. **N357771** – Held origin: **Thailand**\n2. **N359646** – Held origin: **Vietnam**\n3. **N347556** – Held origin: **Vietnam**\n\n*(Note: These results are for informational purposes. Always confirm classification and country of origin with a licensed customs broker or an official CBP binding ruling.)*"}
{"type": "result", "timestamp": "2026-11-21T14:54:44.457Z", "status": "success", "stats": {"total_tokens": 18704, "input_tokens": 18008, "output_tokens": 130, "cached": 6864, "input": 11144, "duration_ms": 8393, "tool_calls": 1, "models": {"gemini-3.8-flash": {"total_tokens": 18704, "input_tokens": 18008, "output_tokens": 130, "cached": 6864, "input": 11144}}}}
```
