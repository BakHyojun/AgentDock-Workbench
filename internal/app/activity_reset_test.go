package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/uvwt/agentdock/internal/activity"
	"github.com/uvwt/agentdock/internal/permission"
)

func localResetContext(t *testing.T) context.Context {
	return activity.WithLocalManagement(t.Context())
}
func assertResetBusy(t *testing.T, r *Runtime) {
	t.Helper()
	_, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true})
	var tool *ToolError
	if !errors.As(err, &tool) || tool.Code != "ACTIVITY_RESET_BUSY" {
		t.Fatal("active work was not protected", err)
	}
}

func TestActivityResetRequiresLocalConfirmationAndBlocksAdmission(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	old := scopeRead(t, r, scopeHost("reset-permission"))
	for _, input := range []struct {
		ctx     context.Context
		confirm bool
	}{{t.Context(), true}, {localResetContext(t), false}} {
		if _, err := r.RuntimeResetActivity(input.ctx, ActivityResetRequest{ConfirmPermanent: input.confirm}); err == nil {
			t.Fatal("unconfirmed or remote reset succeeded")
		}
	}
	if _, err := r.activity.Call(t.Context(), stringArg(old, "call_id")); err != nil {
		t.Fatal("rejected reset removed history", err)
	}
	r.activityResetMu.Lock()
	result, err := r.Call(t.Context(), "list_dir", map[string]any{"path": "."})
	r.activityResetMu.Unlock()
	var tool *ToolError
	if result != nil || !errors.As(err, &tool) || tool.Code != "ACTIVITY_RESET_BUSY" {
		t.Fatal("call entered during reset", result, err)
	}
}

func TestActivityResetProtectsRootAndFinalAdapterEnvelope(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	entered, finish, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(finish) }) })
	go func() {
		_, err := r.callObserved(scopeHost("reset-inflight"), ToolSpec{Name: "read_file", Handler: func(context.Context, *Runtime, map[string]any) (Result, error) {
			close(entered)
			<-finish
			return Result{"text": "actual business result"}, nil
		}}, map[string]any{"path": "fixture.txt"})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("fixture did not enter")
	}
	assertResetBusy(t, r)
	release.Do(func() { close(finish) })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ctx, pending := BeginToolResponse(scopeHost("reset-envelope"))
	result := scopeRead(t, r, ctx)
	assertResetBusy(t, r)
	r.RecordToolResponse(pending, map[string]any{"structuredContent": result})
	r.RecordToolResponse(pending, map[string]any{"structuredContent": result}) // idempotent lease release
	if _, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true}); err != nil {
		t.Fatal("finished envelope left reset blocked", err)
	}
}

func TestActivityResetProtectsPendingApprovalAndAsyncCommand(t *testing.T) {
	t.Run("pending approval", func(t *testing.T) {
		r := executionTestRuntime(t)
		result, err := r.Call(scopeHost("reset-approval"), "exec_command", map[string]any{"cmd": "echo fixture"})
		if err != nil || stringArg(result, "status") != "pending_approval" {
			t.Fatal("fixture did not await approval", result, err)
		}
		assertResetBusy(t, r)
		r.activityResetMu.Lock()
		_, rejected := r.RuntimeApprovalDecision(localResetContext(t), stringArg(result, "approval_id"), "reject", false)
		r.activityResetMu.Unlock()
		var blocked *ToolError
		if !errors.As(rejected, &blocked) || blocked.Code != "ACTIVITY_RESET_BUSY" {
			t.Fatal("approval changed during reset", rejected)
		}
		approval, err := r.permissions.Approval(t.Context(), stringArg(result, "approval_id"))
		if err != nil || approval.Status != "pending" {
			t.Fatal("blocked decision mutated approval", approval, err)
		}
		if _, err := r.RuntimeApprovalDecision(localResetContext(t), stringArg(result, "approval_id"), "reject", false); err != nil {
			t.Fatal(err)
		}
		if _, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true}); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("asynchronous command", func(t *testing.T) {
		r := newRuntimeValidationTestRuntime(t)
		command := "sleep 20"
		if runtime.GOOS == "windows" {
			command = "Start-Sleep -Seconds 20"
		}
		result, err := r.Call(t.Context(), "exec_command", map[string]any{"cmd": command, "execution_mode": "async"})
		if err != nil || stringArg(result, "session_id") == "" {
			t.Fatal(result, err)
		}
		assertResetBusy(t, r)
		call, err := r.activity.Call(t.Context(), stringArg(result, "call_id"))
		if err != nil || activity.CallTerminal(call.Status) {
			t.Fatal("reset stopped the process", call, err)
		}
		if _, err := r.RuntimeCallStop(localResetContext(t), call.CallID); err != nil {
			t.Fatal(err)
		}
		waitExecutionTerminal(t, r, call.CallID)
		deadline := time.Now().Add(5 * time.Second)
		for r.activity.AppendStatistics().ReservedEvents != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if _, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true}); err != nil {
			t.Fatal("completed async command left reset blocked", err)
		}
	})
}

func TestActivityResetWaitsForApprovedCommandSettlement(t *testing.T) {
	r := executionTestRuntime(t)
	binding := activity.Binding{CallID: "call_reset_approval_watcher"}
	if err := r.appendExecution(activity.Event{Binding: binding, Kind: "call.created", Status: "running", ToolName: "exec_command"}); err != nil {
		t.Fatal(err)
	}
	policy, err := r.permissions.Get(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	approval, err := r.permissions.Create(t.Context(), permission.Approval{Binding: binding, Tool: "exec_command", Mode: permission.Rules, PolicyRevision: policy.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := r.permissions.Claim(t.Context(), approval.ID); err != nil || !claimed {
		t.Fatal("fixture not dispatched", claimed, err)
	}
	r.watchApprovalCommand(approval.ID, binding.CallID, "session_fixture")
	assertResetBusy(t, r)
	if err := r.appendExecution(activity.Event{Binding: binding, Kind: "call.completed", Status: "succeeded", ToolName: "exec_command"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		settled, err := r.permissions.Approval(t.Context(), approval.ID)
		if err != nil {
			t.Fatal(err)
		}
		if settled.Status == "succeeded" && r.activityResetMu.TryLock() {
			r.activityResetMu.Unlock()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("command approval settlement did not release history", settled)
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true}); err != nil {
		t.Fatal(err)
	}
	settled, err := r.permissions.Approval(t.Context(), approval.ID)
	if err != nil || settled.Status != "succeeded" {
		t.Fatal("reset lost settled approval", settled, err)
	}
}

func TestActivityResetPreservesTasksSettingsConversationsAndUnattributedCalls(t *testing.T) {
	r := newRuntimeValidationTestRuntime(t)
	a, b := scopeHost("reset-a"), scopeHost("reset-b")
	task := scopeTask(t, r, a, "retained task")
	setActivityDebug(t, r, true)
	project := filepath.Join(r.cfg.AgentDockDefaultDir, "sentinel.txt")
	if err := os.WriteFile(project, []byte("keep project"), 0600); err != nil {
		t.Fatal(err)
	}
	contexts := []context.Context{a, b, t.Context()}
	before := make([]Result, len(contexts))
	for i, ctx := range contexts {
		before[i] = scopeRead(t, r, ctx)
	}
	if _, err := r.RuntimeResetActivity(localResetContext(t), ActivityResetRequest{ConfirmPermanent: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.tasks.Get(task); err != nil {
		t.Fatal("task deleted", err)
	}
	if !r.MCPPresentationSettings().ActivityFullPayloadDebug {
		t.Fatal("reset changed settings")
	}
	if data, err := os.ReadFile(project); err != nil || string(data) != "keep project" {
		t.Fatal("project deleted", err)
	}
	var wg sync.WaitGroup
	for i, ctx := range contexts {
		wg.Go(func() {
			result, err := r.Call(ctx, "list_dir", map[string]any{"path": ".", "max_entries": 1})
			if err != nil {
				t.Error(err)
				return
			}
			if stringArg(result, "conversation_id") != stringArg(before[i], "conversation_id") || stringArg(result, "task_id") != stringArg(before[i], "task_id") {
				t.Error("reset changed immutable identity", result)
			}
		})
	}
	wg.Wait()
	for _, result := range before {
		if _, err := r.activity.Call(t.Context(), stringArg(result, "call_id")); !errors.Is(err, activity.ErrCallNotFound) {
			t.Fatal("old history reopened", err)
		}
	}
}
