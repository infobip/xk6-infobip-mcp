import { NewClient } from "k6/x/infobip_mcp";
import { check } from 'k6';

export const options = {
  thresholds: {
    checks: ['rate==1'],
  },
};

export default function () {
  try {
    NewClient({
      endpoint: "http://127.0.0.1/mcp",
      timeout: 1,
    });
  } catch (e) {
    check(e, {
      "NewClient throws expected error": (err) => err instanceof Error && err.message.includes("connect: connection refused"),
    });
  }
}
