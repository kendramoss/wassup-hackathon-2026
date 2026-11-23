# @undelabs/mcp

A dependency-free stdio MCP server for [Unde Labs](https://undelabs.dev). It is a
thin relay: each JSON-RPC message read from stdin is POSTed to the hosted
Streamable HTTP endpoint at `https://undelabs.dev/mcp`, and every message in the
answer is written back to stdout. The hosted server is stateless, so the relay
keeps no session and holds no product logic of its own.

## Run it

```sh
npx -y @undelabs/mcp
```

No install and no dependencies; Node 20 or later.

## Claude Desktop

```json
{
  "mcpServers": {
    "unde": {
      "command": "npx",
      "args": ["-y", "@undelabs/mcp"],
      "env": { "UNDE_API_KEY": "op_..." }
    }
  }
}
```

## Claude Code

```sh
claude mcp add --env UNDE_API_KEY=op_... --transport stdio unde -- npx -y @undelabs/mcp
```

## Environment

| Variable | Default | Meaning |
|---|---|---|
| `UNDE_API_KEY` | none | Optional API key (`op_...`), sent as `Authorization: Bearer`. Without it calls are anonymous, with per-address limits. |
| `UNDE_BASE_URL` | `https://undelabs.dev` | The relay calls `<base>/mcp`. |

Results are predictions, not rulings. Confirm with a licensed customs broker or a
CBP binding ruling. See [docs/MCP.md](https://github.com/kendramoss/wassup-hackathon-2026/blob/main/docs/MCP.md)
for the tool list and the API-key setup.

## License

MIT.
