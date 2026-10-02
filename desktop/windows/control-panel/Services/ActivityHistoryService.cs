using System.Net;
using System.Net.Http;

namespace AgentDock.ControlPanel;

internal sealed record ActivityResetResult(ulong LatestSeq, long ReclaimedPayloadBytes,
    long RemainingPayloadBytes, long ProtectedPayloadBytes, bool CleanupIncomplete);

internal sealed partial class ActivityClient
{
    internal async Task<ActivityResetResult> ResetActivityHistoryAsync(CancellationToken token)
    {
        token.ThrowIfCancellationRequested();
        try
        {
            return await SendAsync<ActivityResetResult>(HttpMethod.Post,
                "/internal/runtime/execution/history/reset", new { confirm_permanent = true }, token).ConfigureAwait(false);
        }
        catch (HttpRequestException ex) when (ex.StatusCode == HttpStatusCode.Conflict)
        {
            throw new InvalidOperationException(UiText.Get("ExecutionActivityResetBusy"), ex);
        }
    }
}
