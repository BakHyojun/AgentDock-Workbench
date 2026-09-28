using System.Text.Json;

namespace AgentDock.ControlPanel;

public static class InsertionPresentation
{
    public static bool Unconfirmed(string status) => status is "inner_appended" or "outer_forwarded" or "delivery_unknown";

    private static string ReceiptType(JsonElement item)
    {
        var receipt = item.Text("receipt_type");
        return receipt is "" or "none" ? item.Text("acknowledged_by") : receipt;
    }

    public static string State(JsonElement item) => item.Text("status") switch
    {
        "pending" => UiText.Get("InsertionStatePending"),
        "reserved" => UiText.Get("InsertionStateReserved"),
        "inner_appended" => item.Text("delivery_reason") switch
        {
            "awaiting_receiver_receipt" => UiText.Get("InsertionStateAwaitReceiverReceipt"),
            "awaiting_host_receipt" => UiText.Get("InsertionStateAwaitHostReceipt"),
            _ => UiText.Get("InsertionStateAwaitReceipt")
        },
        "outer_forwarded" => UiText.Get("InsertionStateForwarded"),
        "acknowledged" => ReceiptType(item) == "host_context_committed" ? UiText.Get("InsertionStateContextConfirmed") : UiText.Get("InsertionStateReceiverConfirmed"),
        "attached" => UiText.Get("InsertionStateLegacyAttached"),
        "delivery_unknown" => item.Text("delivery_reason") == "receipt_missing_deadline_elapsed" ? UiText.Get("InsertionStateExpiredUnconfirmed") : UiText.Get("InsertionStateUnknown"),
        "target_changed" => UiText.Get("InsertionStateTargetChanged"),
        "expired" => UiText.Get("InsertionStateExpired"),
        "cancelled" => UiText.Get("InsertionStateCancelled"),
        _ => UiText.Get("InsertionStateNotRecorded")
    };

    public static string Reason(JsonElement item) => item.Text("delivery_reason") switch
    {
        "legacy_inner_response_without_receipt" => UiText.Get("InsertionReasonLegacy"),
        "receipt_missing_deadline_elapsed" => UiText.Get("InsertionReasonExpired"),
        "host_receipt_not_negotiated" => UiText.Get("InsertionReasonNoHostReceipt"),
        "awaiting_receiver_receipt" => UiText.Get("InsertionReasonAwaitReceiver"),
        "awaiting_host_receipt" => UiText.Get("InsertionReasonAwaitHost"),
        "awaiting_context_commit" => UiText.Get("InsertionReasonAwaitContext"),
        "process_restarted_before_receipt" => UiText.Get("InsertionReasonRestarted"),
        "inner_response_not_committed" => UiText.Get("InsertionReasonInnerUncommitted"),
        "outer_projection_failed" => UiText.Get("InsertionReasonProjectionFailed"),
        "context_commit_failed" => UiText.Get("InsertionReasonContextFailed"),
        _ => ""
    };

    // Old clients and future fields cannot expose delivery secrets or create a
    // fake executable tool row. The UI keeps only the presentation whitelist.
    public static JsonElement Snapshot(JsonElement item)
    {
        var result = new Dictionary<string, object?> { ["record_kind"] = "insertion" };
        foreach (var key in new[] { "insertion_id", "conversation_id", "task_id", "thread_id", "workspace_id", "text", "status", "created_at", "updated_at", "expires_at", "call_id", "inner_appended_at", "outer_forwarded_at", "acknowledged_at", "acknowledged_by", "receipt_type", "next_retry_at", "outer_call_id", "host_type", "delivery_reason" })
            if (item.Field(key).ValueKind == JsonValueKind.String) result[key == "call_id" ? "inner_call_id" : key] = item.Text(key);
        result["delivery_attempts"] = item.Number("delivery_attempts");
        result["automatic_attempts_remaining"] = item.Number("automatic_attempts_remaining");
        result["total_attempts_remaining"] = item.Number("total_attempts_remaining");
        result["sequence"] = item.Number("sequence");
        result["retry_requested"] = item.Flag("retry_requested");
        result["manual_retry_available"] = item.Flag("manual_retry_available");
        return JsonSerializer.SerializeToElement(result);
    }
}

public sealed partial class ExecutionCallRow
{
    private DateTimeOffset? _insertionNow;
    public bool IsInsertion => _value.Text("record_kind") == "insertion";
    public string InsertionText => IsInsertion ? _value.Text("text") : "";
    public string InsertionPreview => InsertionText.Replace('\r', ' ').Replace('\n', ' ');
    public DateTimeOffset TimelineAt => _value.Date("request_received_at") ?? _value.Date("created_at") ?? DateTimeOffset.MinValue;
    public bool CanRedeliverInsertion => IsInsertion && InsertionPresentation.Unconfirmed(Status) && _value.Flag("manual_retry_available") && !_value.Flag("retry_requested") && _insertionNow is { } now && _value.Date("expires_at") > now;
    public bool CanCancelInsertion => IsInsertion && (Status is "pending" or "target_changed" || InsertionPresentation.Unconfirmed(Status));

    private string InsertionRetryHint
    {
        get
        {
            var details = new List<string>();
            var attempts = _value.Number("delivery_attempts");
            var automatic = _value.Number("automatic_attempts_remaining");
            var total = _value.Number("total_attempts_remaining");
            if (attempts > 0) details.Add(UiText.Format("InsertionScheduledAttempts", attempts));
            if (InsertionPresentation.Unconfirmed(Status))
            {
                if (total == 0) details.Add(UiText.Get("InsertionRetriesExhausted"));
                else if (automatic == 0) details.Add(UiText.Format("InsertionAutomaticRetriesEnded", total));
                else details.Add(UiText.Format("InsertionRetryBudget", automatic, total));
                if (_value.Flag("retry_requested")) details.Add(UiText.Get("InsertionRetrySkipRequested"));
                else if (_value.Date("next_retry_at") is { } next) details.Add(UiText.Get("InsertionNextRetryAt") + next.ToLocalTime().ToString("yyyy-MM-dd HH:mm:ss.fff"));
                if (_value.Flag("manual_retry_available")) details.Add(UiText.Get("InsertionManualRetryNote"));
            }
            return string.Join("\n", details);
        }
    }

    public string InsertionHint => string.Join("\n", new[] { State, InsertionPresentation.Reason(_value), InsertionRetryHint }.Where(value => value.Length > 0));
    public string InsertionDetails => InsertionText + "\n\n" + InsertionHint + UiText.Get("InsertionMessagePrefix") + Id + UiText.Get("InsertionSentPrefix") + TimelineAt.ToLocalTime().ToString("yyyy-MM-dd HH:mm:ss") +
        (_value.Text("inner_call_id").Length > 0 ? UiText.Get("InsertionInnerCallPrefix") + _value.Text("inner_call_id") : "") +
        (_value.Text("outer_call_id").Length > 0 ? UiText.Get("InsertionOuterCallPrefix") + _value.Text("outer_call_id") : "") +
        (_value.Date("acknowledged_at") is { } at ? UiText.Get("InsertionConfirmedPrefix") + at.ToLocalTime().ToString("yyyy-MM-dd HH:mm:ss") : "");

    public static ExecutionCallRow FromInsertion(JsonElement item, DateTimeOffset? now = null)
    {
        var row = new ExecutionCallRow(InsertionPresentation.Snapshot(item));
        row._insertionNow = now;
        return row;
    }

    public void ApplyInsertion(JsonElement item, DateTimeOffset? now)
    {
        if (!IsInsertion || Id != item.Text("insertion_id") || ConversationId != item.Text("conversation_id")) throw new InvalidOperationException("Insertion row identity changed.");
        if (_value.Date("updated_at") > item.Date("updated_at")) return;
        _value = InsertionPresentation.Snapshot(item); _insertionNow = now; Notify();
    }
}
