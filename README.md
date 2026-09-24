# xk6-infobip-mcp

**k6 extension for Model Context Protocol (MCP) integration**

This k6 extension enables performance testing of MCP (Model Context Protocol) servers by providing a JavaScript API for creating MCP clients, calling tools, and managing connections. It is ideal for load testing MCP-based applications and validating MCP server performance under various conditions.


Originally developed to load test [Infobip MCP Servers](https://www.infobip.com/docs/mcp?utm_source=xk6-infobip-mcp-github&utm_medium=referral&utm_campaign=mcp), this extension works with any MCP-compliant server implementation.

## Supported transports and protocol versions

The extension speaks **Streamable HTTP** only, using the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk). Both server modes are supported and need no configuration on the client side:

- **Stateful** servers that issue an `Mcp-Session-Id` on `initialize`. The session is reused for all tool calls and deleted on `closeConnection()`.
- **Stateless** servers that hold no session. Every tool call is an independent `POST`, which is the mode most horizontally scaled MCP gateways run in.

The protocol version is negotiated on `initialize`. The client offers the newest version the SDK knows (currently `2026-07-28`, which includes the stateless protocol extensions) and falls back to older versions if the server requires it. In practice a stateless server ends up on `2026-07-28` and a stateful server on `2025-11-25`, because the newest protocol is defined for stateless HTTP only.

The client mirrors what production MCP clients do on the wire, using the SDK defaults. What that looks like depends on the negotiated protocol:

- **Stateless server, protocol `2026-07-28`:** the standalone SSE `GET` stream was removed from the protocol, so a client session is just the `initialize` `POST` followed by one `POST` per tool call. There is no session ID and no `DELETE`.
- **Stateful server, protocol `2025-11-25` or older:** after `initialize` the client sends the `initialized` notification, opens the standalone SSE stream with a `GET`, and holds it for the life of the client. `closeConnection()` sends a `DELETE` for the session. Servers that offer no stream answer the `GET` with `405 Method Not Allowed`, which is not counted as a failed request.

> **Breaking change in v1.1.0:** the legacy HTTP+SSE transport (spec `2024-11-05`) and the `isSSE` config option were removed. Scripts that still pass `isSSE` will not fail to parse, but the option is ignored and connecting to a legacy SSE-only server will fail. Upgrade the server to Streamable HTTP.

## Example
```javascript file=script.js
import mcp from "k6/x/infobip_mcp";

export const options = {
  vus: 10,
  duration: '30s',
};

export default function () {
  // Create MCP client
  const client = mcp.NewClient({
    endpoint: "https://your-mcp-server.com/mcp",
    timeout: 30,
    headers: {
      "Authorization": "Bearer your-token"
    }
  });

  // Call a tool on the MCP server
  const result = client.callTool("your_tool_name", {
    param1: "value1",
    param2: 42
  });

  console.log("Tool response:", result);

  // Clean up connection
  client.closeConnection();
}
```


## Quick Start

1. **Build a custom k6 binary with xk6-infobip-mcp**  
   Use [xk6](https://github.com/grafana/xk6) to build k6 with this extension:

   ```sh
   go install go.k6.io/xk6/cmd/xk6@latest
   xk6 build --with github.com/infobip/xk6-infobip-mcp
   ```

2. **Write your test script**  
   Use the example above or create your own test script `script.js`.

3. **Run your test**  
   Use your custom k6 binary to run the script:

   ```sh
   ./k6 run script.js
   ```

## API Reference

### NewClient(config)

Creates a new MCP client instance.

**Parameters:**
- `config.endpoint` (string): MCP server endpoint URL
- `config.timeout` (number): Connection timeout in seconds used for connection and tool call
- `config.headers` (object, optional): Custom HTTP headers, typically `Authorization`. Do not set `Content-Type`, `Accept`, `Mcp-Session-Id` or `Mcp-Protocol-Version`; the transport sets those itself and custom values override them.

**Returns:** MCPClient instance

### MCPClient.callTool(toolName, args)

Calls a tool on the MCP server.

**Parameters:**
- `toolName` (string): Name of the tool to call
- `args` (object): Arguments to pass to the tool

**Returns:** Tool response as a string. All text content parts are concatenated. Image, audio and resource parts are skipped. Returns an empty string if the call fails at the transport level; check `mcp_errors` for those.

### MCPClient.closeConnection()

Closes the MCP client connection.

## Metrics

### MCP-Specific Metrics

| Metric Name | Type | Description |
|-------------|------|-------------|
| `mcp_call_duration` | Trend | Duration of individual MCP tool calls in milliseconds. Use this to analyze response times and identify slow operations. |
| `mcp_calls` | Counter | Total number of MCP tool calls made during the test. Helps track the volume of operations executed. |
| `mcp_success` | Rate | Success rate of MCP operations in percentage. A high rate indicates reliable server performance. |
| `mcp_errors` | Rate | Error rate of MCP operations in percentage. Monitor this to identify reliability issues with your MCP server. |

### HTTP Metrics

Since MCP communication happens over HTTP, standard k6 HTTP metrics are also collected:

| Metric Name | Type | Description |
|-------------|------|-------------|
| `http_req_duration` | Trend | Duration of HTTP requests to the MCP server in milliseconds. Includes connection time, sending, waiting, and receiving. |
| `http_reqs` | Counter | Total number of HTTP requests made to the MCP server. Each MCP operation typically results in one or more HTTP requests. |
| `http_req_failed` | Rate | Rate of failed HTTP requests in percentage (status codes ≥ 400). A `404` or `405` on the session `DELETE` sent by `closeConnection()`, and a `405` on the standalone SSE `GET`, are not counted as failures, because stateless servers and expired sessions legitimately answer that way. |

### Metric Tags

MCP metrics (`mcp_*`) carry:

- **tool**: Name of the tool that was called. Use this in thresholds, e.g. `mcp_call_duration{tool:search_articles}`.
- **method**: Same value as `tool`. Kept for backwards compatibility with existing dashboards; prefer `tool`.

HTTP metrics (`http_*`) carry the standard k6 tags:

- **method**: HTTP method used (GET, POST, DELETE)
- **url**: The MCP server endpoint URL
- **status**: HTTP response status code

## Performance notes

The extension is designed to add as little overhead as possible on top of the MCP server under test.

- **Shared connection pool per VU.** Clients use the HTTP transport k6 already owns for the virtual user, the same one the built-in `k6/http` module uses. Sockets are kept alive and reused across clients and iterations, and released at VU teardown. The pool honours the standard k6 options `--no-connection-reuse`, `--no-vu-connection-reuse`, `batch`, `batchPerHost`, proxies and TLS settings.
- **HTTP/2 when the server offers it.** Against a TLS endpoint that negotiates HTTP/2, all of a VU's tool calls are multiplexed over one connection, matching what production MCP clients do. Pass `--http-debug` or set `K6_HTTP_DEBUG` to inspect the traffic.
- **Create one client per VU.** `NewClient` sends an `initialize` request. Creating a client inside the default function makes every iteration pay that round trip, and against a stateless server it doubles the request count. Keep the client in a module-level variable and create it lazily, as shown in `examples/mcp.js`. Create the client per iteration only when the connection handshake itself is what you want to load test.
- **Batched metric samples.** Each tool call and each HTTP request emits its samples as a single batch to the k6 metrics pipeline, which keeps the per-call overhead low at high request rates.

## Contribute

If you wish to contribute to this project, please start by reading the [Contributing Guidelines](CONTRIBUTING.md).
