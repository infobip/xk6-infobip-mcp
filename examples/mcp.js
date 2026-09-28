import { check, sleep } from "k6";
import mcp from "k6/x/infobip_mcp";
import { randomIntBetween } from "https://jslib.k6.io/k6-utils/1.2.0/index.js";

export const options = {
  scenarios: {
    llm_spike_test: {
      executor: "ramping-vus",
      stages: [
        { duration: "30s", target: 100 },
        { duration: "30s", target: 100 },
        { duration: "10s", target: 0 },
      ],
      gracefulStop: "5s",
    },
  },
};

const STEPS = [
  {
    step: 1,
    tool: "search_articles",
    args: {
      query: "Cool MCP servers",
    },
  },
  {
    step: 2,
    tool: "get_article_content",
    args: {
      id: "art_001",
    },
  },
];

// Module-level state is per VU in k6. Creating the client lazily here means
// each VU runs `initialize` once and then reuses the session for every
// iteration, which is how production MCP clients behave and avoids paying
// the handshake on every iteration.
let mcpClient = null;

function getClient() {
  if (mcpClient === null) {
    // Works against both stateful and stateless Streamable HTTP servers; the
    // transport handles session negotiation. Only send headers the server
    // needs (e.g. auth). Content-Type, Accept and the Mcp-* headers are
    // managed by the transport.
    mcpClient = mcp.NewClient({
      endpoint: "http://localhost:8080/mcp",
      timeout: 60,
      headers: {
        Authorization: `App ${__ENV.API_KEY}`,
      },
    });
  }
  return mcpClient;
}

export default function () {
  const client = getClient();

  for (const stepConfig of STEPS) {
    const res = client.callTool(stepConfig.tool, stepConfig.args);
    check(res, {
      "result is not empty": (r) => r !== "",
    });

    if (stepConfig.step < STEPS.length) {
      sleep(randomIntBetween(5, 10));
    }
  }
}

// If you want to measure the connection handshake as part of the load
// instead, drop getClient() and create/close a client inside default():
//
//   const client = mcp.NewClient({...});
//   client.callTool(...);
//   client.closeConnection();
