package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	protocol "github.com/uvwt/agentdock-protocol"
	"github.com/uvwt/agentdock/internal/config"
	"github.com/uvwt/agentdock/internal/permission"
)

// Exercise the production stateless HTTP adapter with an external MCP server.
// Keep the original host descriptors until after the disabled calls: refreshing
// presentation must not become a prerequisite for tool execution.
func TestDisplayToggleDynamicMCPWithoutHostRefresh(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	started, release := make(chan struct{}), make(chan struct{})
	var discoveries, calls atomic.Int32
	upstreamSDK := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "display-fixture", Version: "1"}, nil)
	upstreamSDK.AddReceivingMiddleware(func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, request mcpsdk.Request) (mcpsdk.Result, error) {
			if method == "initialize" || method == "tools/list" {
				discoveries.Add(1)
			}
			return next(ctx, method, request)
		}
	})
	upstreamSDK.AddTool(&mcpsdk.Tool{Name: "echo", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"mode": map[string]any{"type": "string"}}}},
		func(ctx context.Context, request *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			calls.Add(1)
			var args map[string]any
			if err := json.Unmarshal(request.Params.Arguments, &args); err != nil {
				return nil, err
			}
			if args["mode"] == "slow" {
				close(started)
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			return &mcpsdk.CallToolResult{
				IsError:           args["mode"] == "error",
				Content:           []mcpsdk.Content{&mcpsdk.TextContent{Text: "fixture result"}},
				StructuredContent: map[string]any{"mode": args["mode"]},
				Meta:              mcpsdk.Meta{"ui": map[string]any{"resourceUri": "ui://fixture/output.html"}, "vendor/result": "kept"},
			}, nil
		})
	upstream := httptest.NewServer(mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return upstreamSDK }, &mcpsdk.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	t.Cleanup(upstream.Close)
	h := newMCPAppTestHarness(t, config.Config{AgentDockHome: t.TempDir(), AgentDockDefaultDir: t.TempDir()})
	if _, err := h.runtime.RuntimePermissionsUpdate(ctx, permission.Change{Scope: "global", Mode: permission.Full, ExpectedRevision: 1, ConfirmFull: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.runtime.Call(ctx, "mcp_manage", map[string]any{"action": "add", "name": "display", "description": "isolated display toggle fixture", "transport": "streamable_http", "url": upstream.URL}); err != nil {
		t.Fatal(err)
	}
	hostHTTP := httptest.NewServer(h.server.HTTPHandler())
	t.Cleanup(hostHTTP.Close)
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "cached-host-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcpsdk.StreamableClientTransport{Endpoint: hostHTTP.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	cached, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	cachedURI := ""
	for _, tool := range cached.Tools {
		if tool.Name == "mcp_tool_call" {
			cachedURI, _ = asMap(tool.Meta["ui"])["resourceUri"].(string)
		}
	}
	if cachedURI != protocol.DynamicMCPUIResourceURI {
		t.Fatal("host did not start with the dynamic MCP UI binding")
	}
	call := func(mode string) (*mcpsdk.CallToolResult, error) {
		return session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "mcp_tool_call", Arguments: map[string]any{"name": "display:echo", "arguments": map[string]any{"mode": mode}}})
	}
	assertResult := func(result *mcpsdk.CallToolResult, err error, mode string, disabled bool) {
		t.Helper()
		if err != nil || result == nil || result.IsError != (mode == "error") {
			t.Fatalf("display policy changed the execution outcome: %#v %v", result, err)
		}
		remote := asMap(asMap(result.StructuredContent)["result"])
		if asMap(remote["structuredContent"])["mode"] != mode || len(result.Content) == 0 {
			t.Fatalf("external result lost: %#v", result)
		}
		text, ok := result.Content[0].(*mcpsdk.TextContent)
		if !ok || text.Text != "fixture result" {
			t.Fatal("external text content changed")
		}
		meta := asMap(remote["_meta"])
		if meta["vendor/result"] != "kept" {
			t.Fatal("external non-UI metadata lost")
		}
		if disabled && (asMap(meta["ui"])["resourceUri"] != nil || asMap(result.Meta["ui"])["resourceUri"] != nil || result.Meta["openai/outputTemplate"] != nil) {
			t.Fatal("disabled response advertised a UI template")
		}
	}
	result, err := call("initial")
	assertResult(result, err, "initial", false)
	before := discoveries.Load()
	completed := make(chan struct {
		result *mcpsdk.CallToolResult
		err    error
	}, 1)
	go func() {
		result, err := call("slow")
		completed <- struct {
			result *mcpsdk.CallToolResult
			err    error
		}{result, err}
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("external call did not start", ctx.Err())
	}
	disabled := false
	_, toggleErr := h.runtime.RuntimeUpdateDisplaySettings(ctx, config.DisplayChange{ExpectedRevision: 1, ChatGPTMCPUIEnabled: &disabled})
	close(release)
	if toggleErr != nil {
		t.Fatal(toggleErr)
	}
	select {
	case response := <-completed:
		assertResult(response.result, response.err, "slow", true)
	case <-ctx.Done():
		t.Fatal("in-flight result not delivered", ctx.Err())
	}
	for _, mode := range []string{"success", "error"} {
		result, err = call(mode)
		assertResult(result, err, mode, true)
	}
	if discoveries.Load() != before || calls.Load() != 4 {
		t.Fatalf("display toggle reconnected the external server or lost calls: discoveries=%d before=%d calls=%d", discoveries.Load(), before, calls.Load())
	}
	legacy, err := session.ReadResource(ctx, &mcpsdk.ReadResourceParams{URI: cachedURI})
	if err != nil || len(legacy.Contents) != 1 || legacy.Contents[0].Text != disabledTemplateHTML {
		t.Fatalf("cached host template read failed during grace: %#v %v", legacy, err)
	}
	// Explicit host refresh now observes the current directory on the same client.
	current, err := session.ListTools(ctx, nil)
	if err != nil || len(current.Tools) != len(cached.Tools) {
		t.Fatalf("refresh lost business tools: %#v %v", current, err)
	}
	for _, tool := range current.Tools {
		if asMap(tool.Meta["ui"])["resourceUri"] != nil {
			t.Fatalf("refreshed host still sees UI: %s", tool.Name)
		}
	}
	contextResult, err := session.CallTool(ctx, &mcpsdk.CallToolParams{Name: "agentdock_context"})
	if err != nil || contextResult.IsError || contextResult.StructuredContent == nil {
		t.Fatalf("disabled agentdock_context failed: %#v %v", contextResult, err)
	}
}
