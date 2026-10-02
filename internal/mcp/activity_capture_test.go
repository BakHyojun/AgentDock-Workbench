package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/uvwt/agentdock/internal/activity"
	"github.com/uvwt/agentdock/internal/config"
)

func TestActivityCaptureBothAdaptersKeepLargeBusinessResponse(t *testing.T) {
	for _, adapter := range []string{"SDK", "Invoke"} {
		t.Run(adapter, func(t *testing.T) {
			h := newMCPAppTestHarness(t, config.Config{AgentDockHome: t.TempDir(), AgentDockDefaultDir: t.TempDir()})
			body := strings.Repeat("accessibility fixture\n", 16000)
			// AGENTS.md is already exempt from ordinary output truncation. This
			// fixture exercises the real adapter without changing that contract.
			path := filepath.Join(h.runtime.Config().AgentDockDefaultDir, "AGENTS.md")
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			host := "history-" + adapter
			ctx := activity.WithSource(t.Context(), activity.Source{Principal: "local", Provider: "openai", Namespace: "native", HostConversationID: host})
			var firstID string
			for _, debug := range []bool{false, true, false} {
				if _, err := h.runtime.RuntimeUpdateDisplaySettings(t.Context(), config.DisplayChange{ExpectedRevision: h.runtime.MCPPresentationSettings().Revision, ActivityFullPayloadDebug: &debug}); err != nil {
					t.Fatal(err)
				}
				args := map[string]any{"path": path, "max_bytes": len(body)}
				var envelope map[string]any
				if adapter == "SDK" {
					result, err := h.session.CallTool(t.Context(), &mcpsdk.CallToolParams{Name: "read_file", Arguments: args, Meta: mcpsdk.Meta{"openai/session": host}})
					if err != nil || result.IsError {
						t.Fatal("SDK delivery failed after policy change", err)
					}
					envelope = normalizedEnvelope(t, result)
				} else {
					result, err := h.server.Invoke(ctx, "read_file", args)
					if err != nil {
						t.Fatal(err)
					}
					envelope = normalizedEnvelope(t, result)
				}
				structured := asMap(envelope["structuredContent"])
				if structured["content"] != body || envelope["isError"] == true {
					t.Fatal("history policy changed the full business response")
				}
				id, _ := structured["call_id"].(string)
				if id == "" || id == firstID {
					t.Fatal("adapter lost independent root identity")
				}
				firstID = id
				replay, err := activity.New(filepath.Join(h.runtime.Config().AgentDockHome, "tasks", "activity"), activity.Options{})
				if err != nil {
					t.Fatal(err)
				}
				detail, err := replay.Call(t.Context(), id)
				if err != nil {
					t.Fatal(err)
				}
				want := "preview_only"
				if debug {
					want = "complete"
				}
				if detail.Status != "succeeded" || detail.RPCStatus != "succeeded" || detail.Response.State != want || (detail.Response.Ref != "") != debug {
					t.Fatalf("adapter mixed execution and persistence: %+v", detail)
				}
			}
		})
	}
}
