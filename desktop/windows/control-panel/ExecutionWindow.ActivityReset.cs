using System.Windows;

namespace AgentDock.ControlPanel;

public partial class ExecutionWindow
{
    private bool _resettingActivity;

    private async Task ResetActivityHistoryAsync()
    {
        if (_resettingActivity || !ExecutionDialogs.Confirm(this,
            UiText.Get("ExecutionActivityResetTitle"), UiText.Get("ExecutionActivityResetWarning"),
            UiText.Get("ExecutionActivityResetAccept"), scrollExplanation: true)) return;
        _resettingActivity = true;
        try
        {
            var result = await _client.ResetActivityHistoryAsync(_lifetime.Token);
            ClearActivityPresentation();
            // A successful destructive operation remains acknowledged even if
            // the following refresh fails. Never encourage a blind retry.
            await GuardAsync(async () => { await LoadObjectsAsync(); await LoadCallsAsync(false); await RefreshOverviewAsync(); });
            var message = UiText.Format("ExecutionActivityResetDone", result.ReclaimedPayloadBytes, result.RemainingPayloadBytes);
            if (result.ProtectedPayloadBytes > 0) message += "\n" + UiText.Get("ExecutionActivityResetProtected");
            if (result.CleanupIncomplete) message += "\n" + UiText.Get("ExecutionActivityResetIncomplete");
            ShowInfo(UiText.Get("ExecutionActivityResetTitle"), message);
        }
        finally { _resettingActivity = false; }
    }

    private void ClearActivityPresentation()
    {
        _streamEpoch++; _dataEpoch++;
        _streamCancellation?.Cancel();
        _detailCall = null;
        CallDetailsTabs.DataContext = null;
        CloseDetails();
        var updating = _updating;
        _updating = true;
        try { Calls.Clear(); _callsById.Clear(); _managedItems.Clear(); ManagedObjectsList.ItemsSource = null; }
        finally { _updating = updating; }
        _before = 0; _hasOlderCalls = false;
        MoreDataButton.Visibility = Visibility.Collapsed;
        UpdateEmpty();
    }
}
