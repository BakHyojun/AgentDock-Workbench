package app

import (
	"context"
	"time"

	"github.com/uvwt/agentdock/internal/activity"
)

type ActivityResetRequest struct {
	ConfirmPermanent bool `json:"confirm_permanent"`
}

// RuntimeResetActivity is available only to authenticated local management.
// Tasks, conversations, permissions, insertions and project files are preserved.
func (r *Runtime) RuntimeResetActivity(ctx context.Context, request ActivityResetRequest) (activity.ResetResult, error) {
	var result activity.ResetResult
	if !activity.IsLocalManagement(ctx) {
		return result, toolError("LOCAL_USER_REQUIRED", "Activity reset requires local management.", "permission")
	}
	if !request.ConfirmPermanent {
		return result, toolError("CONFIRMATION_REQUIRED", "Confirm permanent removal of Activity history.", "validation")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	busy := func() (activity.ResetResult, error) {
		return result, toolError("ACTIVITY_RESET_BUSY", "Finish tool responses and commands, and resolve pending approvals before resetting Activity history.", "conflict")
	}
	if !r.activityResetMu.TryLock() {
		return busy()
	}
	defer r.activityResetMu.Unlock()
	r.executionMu.Lock()
	defer r.executionMu.Unlock()
	if len(r.activeCalls) != 0 || len(r.pendingCalls) != 0 || r.activity.AppendStatistics().ReservedEvents != 0 || r.command != nil && len(r.command.ActiveConversationBindings()) != 0 {
		return busy()
	}
	r.lifecycleMu.RLock()
	closing := r.closing
	r.lifecycleMu.RUnlock()
	if closing {
		return result, toolError("RUNTIME_CLOSING", "AgentDock runtime is shutting down.", "runtime")
	}
	result, err := r.activity.Reset(ctx, time.Now())
	if err == nil {
		r.sidebarHistory.mu.Lock()
		r.sidebarHistory.entries = nil
		r.sidebarHistory.ids = 0
		r.sidebarHistory.mu.Unlock()
	}
	return result, err
}
