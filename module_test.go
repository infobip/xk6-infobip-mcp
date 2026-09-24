package infobip_mcp

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
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

// countingDialer counts how many TCP connections were opened, so tests can
// assert that the shared VU transport is actually reusing sockets.
type countingDialer struct {
	net.Dialer
	dials atomic.Int64
}

func (d *countingDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	d.dials.Add(1)
	return d.Dialer.DialContext(ctx, network, addr)
}

func newTestRuntime(t *testing.T) (*modulestest.Runtime, *countingDialer) {
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
	dialer := &countingDialer{}
	// Mirror what k6 itself hands to a VU: one shared keep-alive transport.
	transport := &http.Transport{
		DialContext:         dialer.DialContext,
		MaxIdleConns:        20,
		MaxIdleConnsPerHost: 6,
	}
	t.Cleanup(transport.CloseIdleConnections)
	state := &lib.State{
		Options: lib.Options{
			SystemTags: &metrics.DefaultSystemTagSet,
		},
		Samples:        samples,
		Tags:           lib.NewVUStateTags(registry.RootTagSet().WithTagsFromMap(map[string]string{"group": lib.RootGroupPath})),
		BuiltinMetrics: builtinMetrics,
		Dialer:         dialer,
		Transport:      transport,
	}
	runtime.MoveToVUContext(state)

	return runtime, dialer
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
			runtime, _ := newTestRuntime(t)
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

// Test_connectionReuse asserts that repeated create/call/close cycles on a
// stateless server go through the VU's shared transport and reuse sockets,
// instead of opening a fresh connection pool per client.
func Test_connectionReuse(t *testing.T) {
	t.Parallel()

	endpoint := startTestMCPServer(t, &mcp.StreamableHTTPOptions{Stateless: true})
	runtime, dialer := newTestRuntime(t)
	require.NoError(t, runtime.VU.Runtime().Set("ENDPOINT", endpoint))

	const iterations = 50
	script := fmt.Sprintf(`(() => {
		for (let i = 0; i < %d; i++) {
			const c = mcp.NewClient({endpoint: ENDPOINT});
			c.callTool("greet", {"name": "k6"});
			c.closeConnection();
		}
		return true;
	})()`, iterations)

	got, err := runtime.RunOnEventLoop(script)
	require.NoError(t, err)
	require.True(t, got.ToBoolean())

	// 50 clients x 2 requests each = 100 HTTP requests. Sequential calls on a
	// keep-alive pool should need only a handful of sockets.
	dials := dialer.dials.Load()
	require.LessOrEqual(t, dials, int64(5), "expected sockets to be reused, but %d connections were dialed for %d clients", dials, iterations)
}

// Test_initContext asserts that NewClient fails cleanly when called outside
// the VU context, where k6 provides no state or transport.
func Test_initContext(t *testing.T) {
	t.Parallel()

	runtime := modulestest.NewRuntime(t)
	registry := metrics.NewRegistry()
	runtime.VU.InitEnvField = &common.InitEnvironment{
		TestPreInitState: &lib.TestPreInitState{
			Registry:       registry,
			BuiltinMetrics: metrics.RegisterBuiltinMetrics(registry),
			Logger:         logrus.New(),
		},
	}
	m := new(rootModule).NewModuleInstance(runtime.VU)
	require.NoError(t, runtime.VU.Runtime().Set("mcp", m.Exports().Named))

	_, err := runtime.RunOnEventLoop(`mcp.NewClient({endpoint: "http://127.0.0.1:1/mcp"})`)
	require.ErrorContains(t, err, "VU context")
}
