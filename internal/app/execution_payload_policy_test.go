package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/uvwt/agentdock/internal/activity"
	"github.com/uvwt/agentdock/internal/config"
)

func setActivityDebug(t *testing.T, r *Runtime, enabled bool) {
	t.Helper()
	result, err := r.RuntimeUpdateDisplaySettings(t.Context(), config.DisplayChange{ExpectedRevision: r.MCPPresentationSettings().Revision, ActivityFullPayloadDebug: &enabled})
	if err != nil || result["activity_full_payload_debug"] != enabled {
		t.Fatalf("debug preference not applied: %v %v", result, err)
	}
}

func TestLargeHistoryPreservesConcurrentResultsAndIdentity(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	body := strings.Repeat("CUA accessibility fixture\n", 100000)
	spec := ToolSpec{Name: "read_file", Handler: func(context.Context, *Runtime, map[string]any) (Result, error) {
		return Result{"text": body}, nil
	}}
	contexts := []context.Context{scopeHost("large-a"), scopeHost("large-b"), t.Context()}
	results := make([]Result, len(contexts))
	errors := make([]error, len(contexts))
	var wg sync.WaitGroup
	for i, ctx := range contexts {
		wg.Go(func() { results[i], errors[i] = r.callObserved(ctx, spec, map[string]any{"path": "sample.txt"}) })
	}
	wg.Wait()
	seenConversations := map[string]bool{}
	for i := range contexts {
		if errors[i] != nil || stringArg(results[i], "text") != body {
			t.Fatal("history policy altered tool delivery", errors[i])
		}
		call, err := r.activity.Call(t.Context(), stringArg(results[i], "call_id"))
		if err != nil {
			t.Fatal(err)
		}
		if call.Status != "succeeded" || call.RPCStatus != "succeeded" || call.TaskID != "" || call.Response == nil || call.Response.State != "preview_only" || call.Response.Ref != "" {
			t.Fatalf("execution, storage or task state conflated: %+v", call)
		}
		if i == 2 {
			if call.ConversationID != "" {
				t.Fatal("unattributed call borrowed a conversation")
			}
		} else if call.ConversationID == "" || seenConversations[call.ConversationID] {
			t.Fatal("concurrent calls lost distinct source identity")
		} else {
			seenConversations[call.ConversationID] = true
		}
	}
	replay, err := activity.New(filepath.Join(r.cfg.AgentDockHome, "tasks", "activity"), activity.Options{})
	if err != nil {
		t.Fatal(err)
	}
	page, err := replay.ReadCallPayload(t.Context(), stringArg(results[0], "call_id"), "response", 0, 32768)
	if err != nil || page.Payload.State != "preview_only" || page.HasMore || page.Text == "" {
		t.Fatal("large response preview could not be reopened", err)
	}
}

func TestLargePreviewDoesNotHideToolFailure(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	body := strings.Repeat("failure detail", 25000)
	result := outputCall(t, r, scopeHost("large-failure"), "read_file", map[string]any{"path": "sample.txt"}, Result{"isError": true, "text": body})
	call, err := r.activity.Call(t.Context(), stringArg(result, "call_id"))
	if err != nil || call.Status != "failed" || call.RPCStatus != "failed" || call.Response.State != "preview_only" || result["isError"] != true || stringArg(result, "text") != body {
		t.Fatalf("preview policy hid execution failure: %+v %v", call, err)
	}
}

func TestActivityDebugSnapshotIncludesFinalAdapterEnvelope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, adapter := range []bool{false, true} {
			r := newRuntimeValidationTestRuntime(t)
			setActivityDebug(t, r, enabled)
			body := strings.Repeat("x", activity.MaxHistoryFullPayloadBytes+1)
			ctx := scopeHost("debug-snapshot")
			var response *ToolResponse
			if adapter {
				ctx, response = BeginToolResponse(ctx)
			}
			spec := ToolSpec{Name: "read_file", Handler: func(context.Context, *Runtime, map[string]any) (Result, error) {
				setActivityDebug(t, r, !enabled)
				return Result{"text": body}, nil
			}}
			result, err := r.callObserved(ctx, spec, map[string]any{"path": "sample.txt"})
			if err != nil || stringArg(result, "text") != body {
				t.Fatal("live setting toggle changed result", err)
			}
			if adapter {
				r.RecordToolResponse(response, map[string]any{"structuredContent": result, "isError": false})
			}
			call, err := r.activity.Call(t.Context(), stringArg(result, "call_id"))
			if err != nil {
				t.Fatal(err)
			}
			want := "preview_only"
			if enabled {
				want = "complete"
			}
			if call.Response.State != want || (call.Response.Ref != "") != enabled || call.RPCStatus != "succeeded" {
				t.Fatalf("in-flight policy changed: enabled=%v adapter=%v %+v", enabled, adapter, call.Response)
			}
			if call.Request.State != "complete" || call.Request.Ref == "" {
				t.Fatal("normal small requests were not preserved")
			}
		}
	}
}

func TestNormalHistoryKeepsLargeContinuationSource(t *testing.T) {
	r := executionTestRuntime(t)
	ctx := scopeHost("large-continuation")
	setOutputBudget(t, r, true, 1000)
	body := strings.Repeat("ordinary output\n", 22000)
	result := outputCall(t, r, ctx, "read_file", map[string]any{"path": "sample.txt"}, Result{"content": body})
	policy := result["output_policy"].(map[string]any)
	args := policy["continue_read"].(map[string]any)["arguments"].(map[string]any)
	call, err := r.activity.Call(t.Context(), stringArg(result, "call_id"))
	if err != nil || call.OutputSource == nil || call.OutputSource.Ref == "" || call.OutputSource.Bytes <= activity.MaxHistoryFullPayloadBytes {
		t.Fatal("large continuation source was reduced to preview", err)
	}
	// Direct descriptor reading checks the entire preserved JSON source, beyond preview.
	var all strings.Builder
	var offset int64
	for {
		page, err := r.activity.ReadCallPayload(t.Context(), call.CallID, "source", offset, 262144)
		if err != nil {
			t.Fatal(err)
		}
		all.WriteString(page.Text)
		if !page.HasMore {
			break
		}
		offset = page.NextOffset
	}
	var source map[string]string
	if err := json.Unmarshal([]byte(all.String()), &source); err != nil || source["content"] != body {
		t.Fatal("history policy discarded ordinary source bytes", err)
	}
	page, err := r.Call(ctx, "read_file", args)
	if err != nil || stringArg(page, "content") == "" {
		t.Fatal("authenticated continuation stopped working", err)
	}
}
