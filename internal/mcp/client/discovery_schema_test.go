package client

import (
	"context"
	"errors"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDiscoveryInputSchemaShapeIsValidatedBeforeSDKFiltering(t *testing.T) {
	var typedNil map[string]any
	for _, test := range []struct {
		name   string
		schema any
		valid  bool
	}{
		{"nil", nil, false},
		{"typed-nil-map", typedNil, false},
		{"array", []any{}, false},
		{"boolean", false, false},
		{"string", "object", false},
		{"number", 7, false},
		{"empty-object", map[string]any{}, true},
		{"object", map[string]any{"type": "object", "properties": map[string]any{}}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := newStreamableHTTPClient(ServerConfig{Name: "schema-fixture"})
			original := &mcpsdk.ListToolsResult{Tools: []*mcpsdk.Tool{{Name: "original-name", InputSchema: test.schema}}}
			handler := client.validateDiscoveryResponse(func(context.Context, string, mcpsdk.Request) (mcpsdk.Result, error) {
				return original, nil
			})
			got, err := handler(t.Context(), "tools/list", nil)
			if test.valid {
				if err != nil || got != original || original.Tools[0].Name != "original-name" {
					t.Fatalf("valid schema/result changed: %v %v", got, err)
				}
				return
			}
			var failure *Error
			if got != nil || !errors.As(err, &failure) || failure.Code != "MCP_INVALID_RESPONSE" {
				t.Fatalf("invalid schema was not a structured response error: %v %v", got, err)
			}
		})
	}
}

func TestDiscoveryMiddlewareDoesNotValidateOtherMethods(t *testing.T) {
	client := newStreamableHTTPClient(ServerConfig{Name: "schema-fixture"})
	original := &mcpsdk.ListToolsResult{} // Deliberately invalid only for tools/list.
	handler := client.validateDiscoveryResponse(func(context.Context, string, mcpsdk.Request) (mcpsdk.Result, error) {
		return original, nil
	})
	got, err := handler(t.Context(), "tools/call", nil)
	if err != nil || got != original {
		t.Fatalf("discovery middleware changed an unrelated response: %v %v", got, err)
	}
}
