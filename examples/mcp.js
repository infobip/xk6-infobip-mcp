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

export default function () {
  // One client per iteration. Works against both stateful and stateless
  // Streamable HTTP servers; the transport handles session negotiation.
  // Only send headers the server needs (e.g. auth). Content-Type, Accept
  // and the Mcp-* headers are managed by the transport.
  const mcpClient = mcp.NewClient({
    endpoint: "http://localhost:8080/mcp",
    timeout: 60,
    headers: {
      Authorization: `App ${__ENV.API_KEY}`,
    },
  });

  for (const stepConfig of STEPS) {
    // Reuse the same client for all iterations
    let res = mcpClient.callTool(stepConfig.tool, stepConfig.args);
    check(res, {
      "result is not empty": (r) => r !== "",
    });

    if (stepConfig.step < STEPS.length) {
      sleep(randomIntBetween(5, 10));
    }
  }

  mcpClient.closeConnection();
}
