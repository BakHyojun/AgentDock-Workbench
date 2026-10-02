using AgentDock.ControlPanel;
using System.Text;
using System.Text.Json;

internal static class OutputPolicyTests
{
    private static JsonElement Json(object value) => JsonSerializer.SerializeToElement(value);
    public static void Run(Action<bool, string> check)
    {
        var row = new ExecutionCallRow(Json(new { call_id = "source-fixture", updated_seq = 1, response = new { state = "complete", @ref = "returned", bytes = 100, preview = "returned envelope" }, output_source = new { state = "complete", @ref = "retained", bytes = 400000, preview = "original source" } }));
        check(row.ResponsePayloadKind == "source" && row.Output == "original source", "retained source drives output paging");
        var text = string.Concat(Enumerable.Repeat("😀", 100000));
        var budget = new ToolOutputSettings(true, 100000).Budget;
        var page = Json(new { payload = new { @ref = "retained" }, offset = 0, next_offset = Encoding.UTF8.GetByteCount(text), has_more = false, text, unit = "unicode_scalar", limit_chars = 100000, returned_chars = 100000 });
        check(row.ResponsePayload.ApplyPage(page, "retained", false, budget), "100000 supplementary scalar source page applies");
        check(row.Output.Length == 200000 && row.ResponsePayload.Position == UiText.Format("ExecutionPayloadAllSaved", 400000L), "UTF16 length is not the scalar budget and all saved bytes are reported");
        row.Apply(Json(new { call_id = "source-fixture", updated_seq = 2, response = new { state = "complete", @ref = "returned-final", bytes = 600 }, output_source = new { state = "complete", @ref = "retained", bytes = 400000 } }));
        check(row.ResponsePayloadKind == "source" && row.Output == text, "final response envelope cannot replace retained source or loaded page");
        var mixed = "中😀e\u0301\r\n";
        check(ToolOutputSettings.ScalarCount(mixed) == 6 && mixed.EnumerateRunes().Count() == 6, "scalar helper counts combining mark and both CRLF scalars");
        var legacy = new ExecutionCallRow(Json(new { response = new { state = "complete", @ref = "legacy", bytes = 5, preview = "legacy output" } }));
        check(legacy.ResponsePayloadKind == "response" && legacy.Output == "legacy output", "old records retain original response path");
        row.Apply(Json(new { call_id = "source-fixture", updated_seq = 3, output_source = new { state = "partial", @ref = "partial", bytes = 4, reason = "源输出未完整保存" } }));
        check(row.ResponsePayload.ApplyPage(Json(new { payload = new { @ref = "partial" }, offset = 0, next_offset = 4, has_more = false, text = "1234" }), "partial", false), "partial source last page applies");
        check(row.ResponsePayload.Position == UiText.Format("ExecutionPayloadRangePartial", UiText.Format("ExecutionPayloadByteRange", 1L, 4L), 4L) + "；源输出未完整保存", "partial source retains its original storage reason and is never described as complete");
        var preview = new ExecutionCallRow(Json(new { call_id = "preview-fixture", status = "succeeded", rpc_status = "succeeded", response = new { state = "preview_only", bytes = 2700000, preview = "large state preview", truncated = true, reason = "Normal-mode history policy" } }));
        check(preview.ResponsePayload.StateLabel == UiText.Get("ExecutionPayloadPreviewOnly") && preview.Output == "large state preview", "intentional preview is rendered with its own state");
        check(preview.ResponsePayload.Reason == UiText.Get("ExecutionPayloadPreviewOnlyReason") && !preview.ResponsePayload.NeedsLoad && !preview.ResponsePayload.CanReadNext, "preview explains the policy without fabricating full detail pages");
        var unknown = new ExecutionPayloadView("response"); unknown.Describe(default);
        check(unknown.State == "unknown" && unknown.StateLabel != preview.ResponsePayload.StateLabel, "old missing output is distinct from intentional preview");
    }
}
