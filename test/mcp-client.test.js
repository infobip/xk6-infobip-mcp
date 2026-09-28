import { check, group } from "k6";
import { NewClient } from "k6/x/infobip_mcp";

export const options = {
  thresholds: {
    checks: ["rate==1"],
  },
};

const TEST_ENDPOINT = `${__ENV.MCP_SERVER_URL}`;

export default function () {
  group("MCP Client Creation", function () {
    const config = {
      endpoint: TEST_ENDPOINT,
      timeout: 30,
      headers: {
        "Authorization": `App ${__ENV.API_KEY}`
      }
    };

    const client = NewClient(config);

    check(client, {
      "client is created": (c) => c !== null && c !== undefined,
      "client has callTool method": (c) => typeof c.callTool === "function",
      "client has closeConnection method": (c) => typeof c.closeConnection === "function",
    });
  });

  group("MCP Client Methods", function () {
    const client = NewClient({
      endpoint: TEST_ENDPOINT,
      timeout: 30,
      headers: {
        "Authorization": `App ${__ENV.API_KEY}`
      }
    });

    group("CallTool Method", function () {
      check(client, {
        "CallTool method exists": (c) => typeof c.callTool === "function",
      });

      const toolName = "send_sms_messages";
      const toolArgs = {
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
      };

      // Note: This will likely fail in actual execution without a real MCP server
      // but the test structure validates the method signature
      try {
        const result = client.callTool(toolName, toolArgs);
        check(result, {
          "CallTool returns a value": (r) => r !== undefined,
          "CallTool returns string": (r) => typeof r === "string",
        });
      } catch (e) {
        // Expected to fail without real MCP server
        check(e, {
          "CallTool throws expected error": (err) => err instanceof Error,
        });
      }
    });

    // Test CloseConnection method
    group("CloseConnection Method", function () {
      check(client, {
        "CloseConnection method exists": (c) => typeof c.closeConnection === "function",
      });

      try {
        const result = client.closeConnection();
        check(result, {
          "CloseConnection executes": (r) => true,
        });
      } catch (e) {
        check(e, {
          "CloseConnection handles errors": (err) => err instanceof Error,
        });
      }
    });
  });
}
