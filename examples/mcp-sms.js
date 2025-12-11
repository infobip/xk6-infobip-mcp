/**
 * Infobip MCP SMS Load Testing Example
 *
 * This k6 script demonstrates how to perform load testing on Infobip MCP (Message Control Protocol)
 * servers using the xk6-infobip-mcp extension. The test simulates SMS operations including sending
 * messages and retrieving delivery reports.
 *
 * PREREQUISITES:
 * - Valid Infobip account: https://www.infobip.com/signup
 * - xk6-infobip-mcp extension compiled into k6
 * - Access to Infobip MCP server endpoint
 *
 * REQUIRED ENVIRONMENT VARIABLES:
 * - MCP_SERVER_URL: The Infobip MCP server endpoint URL
 * - API_KEY: Your Infobip API key for authentication
 * - MOBILE_NUMBER: Target mobile number for SMS delivery (in international format)
 *
 * EXAMPLE USAGE:
 * ```bash
 * export MCP_SERVER_URL="https://mcp.infobip.com/sms"
 * export API_KEY="your-api-key-here"
 * export MOBILE_NUMBER="+1234567890"
 * k6 run examples/mcp-sms.js
 * ```
 *
 * TEST SCENARIO:
 * - Ramps up to 100 virtual users over 30 seconds
 * - Maintains 100 VUs for 30 seconds
 * - Ramps down to 0 VUs over 10 seconds
 * - Each VU performs a 2-step SMS workflow:
 *   1. Sends an SMS message using send_sms_messages tool
 *   2. Retrieves delivery reports using get_sms_message_delivery_reports tool
 */

import { check, sleep } from 'k6';
import mcp from "k6/x/infobip_mcp";
import { randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';

export const options = {
  scenarios: {
    llm_spike_test: {
      executor: 'ramping-vus',
      stages: [
        { duration: '30s', target: 100 },
        { duration: '30s', target: 100 },
        { duration: '10s', target: 0 },
      ],
      gracefulStop: '5s',
    },
  },
};

const STEPS = [
  {
    step: 1,
    tool: 'send_sms_messages',
    args: {
      "messages": [
        {
          "sender": "Infobip MCP k6",
          "destinations": [
            {
              "to": `${__ENV.MOBILE_NUMBER}`
            }
          ],
          "content": {
            "text": "Hi! I'm Infobip MCP k6 extension!"
          }
        }
      ]
    }
  },
  {
    step: 2,
    tool: 'get_sms_message_delivery_reports',
    args: {
      limit: 1
    }
  },
];

export default function () {
  // Initialize client only on first iteration for this VU
  const mcpClient = mcp.NewClient({
    endpoint: `${__ENV.MCP_SERVER_URL}`,
    isSSE: false,
    timeout: 60,
    headers: {
      "Authorization": `App ${__ENV.API_KEY}`,
      "Content-Type": "application/json",
      "Accept": "application/json, text/event-stream"
    }
  });


  for (const stepConfig of STEPS) {
    // Reuse the same client for all iterations
    let res = mcpClient.callTool(stepConfig.tool, stepConfig.args);
    check(res, {
      'result is not empty': (r) => r !== "",
    });

    if (stepConfig.step < STEPS.length) {
      sleep(randomIntBetween(5, 10));
    }
  }

  mcpClient.closeConnection();
}
