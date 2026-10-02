using System.ComponentModel;
using System.IO;
using System.Text.Json;
using System.Text;

namespace AgentDock.ControlPanel;

public sealed class ExecutionPayloadView(string kind) : INotifyPropertyChanged
{
    public const int PageBytes = 32768;
    private readonly List<long> _previous = [];
    private bool _loaded;
    private bool _knownBytes;
    private int _displayedChars;
    private PayloadPageBudget? _loadedBudget;
    private string _readError = "", _storageReason = "";
    public event PropertyChangedEventHandler? PropertyChanged;
    public string State { get; private set; } = "unknown";
    public string Reference { get; private set; } = "";
    public string Reason => _readError.Length > 0 ? _readError : _storageReason;
    public string Text { get; private set; } = "";
    public long TotalBytes { get; private set; }
    public long Lines { get; private set; }
    public long Offset { get; private set; }
    public long NextOffset { get; private set; }
    public bool HasMore { get; private set; }
    public bool HasPrevious => _previous.Count > 0;
    public bool NeedsLoad => Reference.Length > 0 && !_loaded;
    public bool Busy { get; private set; }
    public bool CanReadNext => HasMore && !Busy;
    public bool CanReadPrevious => HasPrevious && !Busy;
	public bool HasReadError => _readError.Length > 0 && Reference.Length > 0;
    public string Position
    {
        get
        {
            if (State == "unknown") return UiText.Get("ExecutionPayloadMissingBody");
            if (Reference.Length == 0) return Reason.Length > 0 ? StateLabel + "：" + Reason : StateLabel;
            if (!_knownBytes) return UiText.Format("ExecutionPayloadUnknownLength", _displayedChars);
            if (TotalBytes == 0) return State == "streaming" ? UiText.Get("ExecutionPayloadZeroStreaming") : UiText.Get("ExecutionPayloadZero");
            if (!_loaded) return State == "partial" ? UiText.Format("ExecutionPayloadPreviewPartial", TotalBytes) : UiText.Format("ExecutionPayloadPreviewSaved", TotalBytes);
            if (State == "complete" && Offset == 0 && NextOffset == TotalBytes && !HasMore) return UiText.Format("ExecutionPayloadAllSaved", TotalBytes);
            var range = NextOffset > Offset ? UiText.Format("ExecutionPayloadByteRange", Offset + 1, NextOffset) : UiText.Get("ExecutionPayloadEmptyPage");
            return State switch
            {
                "streaming" => UiText.Format("ExecutionPayloadRangeStreaming", range, TotalBytes),
                "partial" => UiText.Format("ExecutionPayloadRangePartial", range, TotalBytes) + (_storageReason.Length > 0 ? "；" + _storageReason : ""),
                _ => UiText.Format("ExecutionPayloadRangeTotal", range, TotalBytes)
            };
        }
    }
    public string StateLabel => State switch
    {
        "pending" => kind == "request" ? UiText.Get("ExecutionRequestSaving") : UiText.Get("ExecutionOutputWaiting"),
        "streaming" => UiText.Get("ExecutionOutputStreaming"), "complete" => UiText.Get("ExecutionPayloadSaved"), "partial" => UiText.Get("ExecutionOutputPartial"),
        "preview_only" => UiText.Get("ExecutionPayloadPreviewOnly"),
        "not_stored" => UiText.Get("NotSaved"), "internal" => UiText.Get("ExecutionInternalCall"), _ => UiText.Get("ExecutionLegacyRecord")
    };

    public void Describe(JsonElement descriptor, string parentCallId = "")
    {
        var present = descriptor.ValueKind == JsonValueKind.Object;
        var reference = descriptor.Text("ref");
        var reset = Reference != reference;
        State = present ? descriptor.Text("state", "unknown") : "unknown";
        _storageReason = State == "preview_only" ? UiText.Get("ExecutionPayloadPreviewOnlyReason") : descriptor.Text("reason");
        _knownBytes = descriptor.OptionalNumber("bytes") is >= 0;
        TotalBytes = descriptor.Number("bytes"); Lines = descriptor.Number("lines");
        Reference = reference;
        if (reset) { _readError = ""; _loaded = false; _loadedBudget = null; _previous.Clear(); Offset = NextOffset = 0; HasMore = false; }
        if (!_loaded)
        {
            Text = descriptor.Text("preview");
            _displayedChars = ToolOutputSettings.ScalarCount(Text);
            if (Text.Length == 0 && Reference.Length == 0)
                Text = Reason.Length > 0 ? Reason : !present || State == "unknown" ? UiText.Format("ExecutionPayloadLegacyMissing", UiText.Get(kind == "request" ? "ExecutionRequest" : "ExecutionOutput")) : StateLabel;
        }
        if (!present && parentCallId.Length > 0) { State = "internal"; Text = UiText.Format("ExecutionPayloadInternalNotice", parentCallId); }
        Notify();
    }

    public long RequestedOffset(bool previous)
    {
        if (previous) return _previous.Count > 0 ? _previous[^1] : 0;
        return _loaded && HasMore ? NextOffset : 0;
    }
    public void SetBusy(bool busy) { Busy = busy; Notify(); }
    public bool NeedsBudgetReload(PayloadPageBudget budget) => _loaded && _loadedBudget != budget;
    public bool ApplyPage(JsonElement page, string expectedReference, bool previous, PayloadPageBudget? requestedBudget = null)
    {
        if (Reference != expectedReference || page.Field("payload").Text("ref") != expectedReference) return false;
        var budget = requestedBudget ?? PayloadPageBudget.BytesOnly;
        var offset = page.Number("offset");
        var next = page.Number("next_offset");
        var text = page.Text("text");
        var scalars = ToolOutputSettings.ScalarCount(text);
        if (offset < 0 || next < offset || _knownBytes && next > TotalBytes || next - offset > budget.MaxBytes || Encoding.UTF8.GetByteCount(text) != next - offset)
            throw new InvalidDataException(UiText.Get("ExecutionPayloadInvalidBoundary"));
        if (budget.LimitChars > 0 && (scalars > budget.LimitChars || page.Text("unit") != "unicode_scalar" || page.Number("limit_chars") != budget.LimitChars || page.Number("returned_chars") != scalars))
            throw new InvalidDataException(UiText.Get("ExecutionPayloadCharacterMismatch"));
        if (page.Flag("has_more") && next <= offset) throw new InvalidDataException(UiText.Get("ExecutionPayloadNoProgress"));
        if (NeedsBudgetReload(budget))
        {
            if (offset != 0) throw new InvalidDataException(UiText.Get("ExecutionPayloadBudgetRestart"));
            _previous.Clear(); _loaded = false;
        }
        if (_loaded && offset != Offset)
        {
            if (previous && _previous.Count > 0) _previous.RemoveAt(_previous.Count - 1);
            else if (!previous) _previous.Add(Offset);
        }
        Offset = offset; NextOffset = next; HasMore = page.Flag("has_more");
        Text = text; _displayedChars = scalars; _loaded = true; _loadedBudget = budget; _readError = "";
        Notify(); return true;
    }
    public void ReadFailed(string reason)
    {
        _readError = reason;
        if (!_loaded && Text.Length == 0) Text = reason;
        Notify();
    }
    private void Notify() => PropertyChanged?.Invoke(this, new(null));
}
