package infobip_mcp

import (
	"context"
	_ "embed"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"go.k6.io/k6/js/common"
	"go.k6.io/k6/js/modulestest"
	"go.k6.io/k6/lib"
	"go.k6.io/k6/metrics"
)

type Input struct {
	Name string `json:"name" jsonschema:"the name of the person to greet"`
}

type Output struct {
	Greeting string `json:"greeting" jsonschema:"the greeting to tell to the user"`
}

func SayHi(ctx context.Context, req *mcp.CallToolRequest, input Input) (
	*mcp.CallToolResult,
	Output,
	error,
) {
	return nil, Output{Greeting: "Hi " + input.Name}, nil
}

// SayHiWithImage returns a text part followed by a non-text part, to verify
// that the client concatenates text and skips everything else.
func SayHiWithImage(ctx context.Context, req *mcp.CallToolRequest, input Input) (
	*mcp.CallToolResult,
	any,
	error,
) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Hi " + input.Name},
			&mcp.ImageContent{MIMEType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}},
			&mcp.TextContent{Text: "!"},
		},
	}, nil, nil
}

// startTestMCPServer starts a Streamable HTTP MCP server on an ephemeral port
// and returns its endpoint URL. The server is stopped when the test ends.
func startTestMCPServer(t *testing.T, opts *mcp.StreamableHTTPOptions) string {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "greeter", Version: "v1.0.0"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "greet", Description: "say hi"}, SayHi)
	mcp.AddTool(server, &mcp.Tool{Name: "greet_with_image", Description: "say hi with a picture"}, SayHiWithImage)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, opts)

	ts := httptest.NewServer(handler)
	t.Cleanup(func() {
		// Stateful servers hold the standalone SSE stream open for every client
		// that was not closed; drop those connections so Close does not block.
		ts.CloseClientConnections()
		ts.Close()
	})

	return ts.URL
}

func newTestRuntime(t *testing.T) *modulestest.Runtime {
	t.Helper()

	runtime := modulestest.NewRuntime(t)

	logger := logrus.New()
	logger.SetLevel(logrus.InfoLevel)

	registry := metrics.NewRegistry()
	builtinMetrics := metrics.RegisterBuiltinMetrics(registry)

	runtime.VU.InitEnvField = &common.InitEnvironment{
		TestPreInitState: &lib.TestPreInitState{
			Registry:       registry,
			BuiltinMetrics: builtinMetrics,
			Logger:         logger,
		},
	}

	m := new(rootModule).NewModuleInstance(runtime.VU)
	require.NoError(t, runtime.VU.Runtime().Set("mcp", m.Exports().Named))

	runtime.VU.InitEnvField = nil
	samples := make(chan metrics.SampleContainer, 1000)
	state := &lib.State{
		Options: lib.Options{
			SystemTags: &metrics.DefaultSystemTagSet,
		},
		Samples:        samples,
		Tags:           lib.NewVUStateTags(registry.RootTagSet().WithTagsFromMap(map[string]string{"group": lib.RootGroupPath})),
		BuiltinMetrics: builtinMetrics,
		Dialer:         &net.Dialer{},
	}
	runtime.MoveToVUContext(state)

	return runtime
}

func Test_module(t *testing.T) {
	t.Parallel()

	servers := []struct {
		name string
		opts *mcp.StreamableHTTPOptions
	}{
		{name: "stateful", opts: nil},
		{name: "stateless", opts: &mcp.StreamableHTTPOptions{Stateless: true}},
	}

	checks := []struct {
		name  string
		check string
	}{
		{
			name:  "NewClient().callTool()",
			check: `JSON.parse(mcp.NewClient({endpoint: ENDPOINT}).callTool("greet", {"name": "k6"})).greeting === "Hi k6"`,
		},
		{
			name:  "callTool() skips non-text content",
			check: `mcp.NewClient({endpoint: ENDPOINT}).callTool("greet_with_image", {"name": "k6"}) === "Hi k6!"`,
		},
		{
			name: "closeConnection()",
			check: `(() => {
				const c = mcp.NewClient({endpoint: ENDPOINT});
				c.callTool("greet", {"name": "k6"});
				return c.closeConnection() === null || c.closeConnection() === undefined;
			})()`,
		},
	}

	for _, srv := range servers {
		t.Run(srv.name, func(t *testing.T) {
			t.Parallel()

			endpoint := startTestMCPServer(t, srv.opts)
			runtime := newTestRuntime(t)
			require.NoError(t, runtime.VU.Runtime().Set("ENDPOINT", endpoint))

			for _, tt := range checks {
				t.Run(tt.name, func(t *testing.T) {
					got, err := runtime.RunOnEventLoop(tt.check)
					require.NoError(t, err)
					require.True(t, got.ToBoolean(), "check returned false: %s", tt.check)
				})
			}
		})
	}
}
