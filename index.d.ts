/**
 * **Infobip MCP k6 extension**
 *
 * A k6 extension for making MCP (Model Context Protocol) tool calls
 *
 * @module infobip_mcp
 */
export as namespace infobip_mcp;

/**
 * Configuration for MCP client connection.
 *
 * The client always uses the Streamable HTTP transport and works against both
 * stateful (session-based) and stateless MCP servers without extra options.
 * The legacy HTTP+SSE transport is not supported.
 */
export interface ClientConfig {
  /** MCP server endpoint URL */
  endpoint: string;
  /** Connection and tool call timeout in seconds */
  timeout: number;
  /**
   * Custom HTTP headers to include with every request, typically Authorization.
   * Do not set Content-Type, Accept or Mcp-* headers; the transport manages them.
   */
  headers?: Record<string, string>;
}

/**
 * MCP Client for making tool calls to MCP servers
 */
export declare class MCPClient {
  /**
   * Close the MCP client connection. On stateful servers this deletes the
   * session; on stateless servers it is a local no-op.
   *
   * @throws Error if connection cannot be closed
   */
  closeConnection(): void;

  /**
   * Call a tool on the MCP server
   *
   * @param toolName Name of the tool to call
   * @param args Arguments to pass to the tool
   * @returns All text content parts of the response concatenated into one
   *   string. Non-text parts are skipped. Empty string on transport failure.
   * @throws Error if the client was not initialized
   */
  callTool(toolName: string, args: Record<string, any>): string;
}

/**
 * Create a new MCP client instance
 *
 * @param config Configuration for the MCP client
 * @returns A new MCPClient instance
 */
export declare function NewClient(config: ClientConfig): MCPClient;
