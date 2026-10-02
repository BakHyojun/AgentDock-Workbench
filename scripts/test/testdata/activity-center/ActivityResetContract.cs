using System.IO;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.Json;
using AgentDock.ControlPanel;

internal static partial class Program
{
    private static async Task TestActivityResetClientAsync()
    {
        UiText.ApplyPreference("ko-KR");
        var requests = 0;
        using var client = new ActivityClient(_ => Task.FromResult(new ActivityConnection(new Uri("http://127.0.0.1:32123"), "fixture-only-token")),
            new ResetHandler(async (request, token) =>
            {
                requests++;
                Require(request.Method == HttpMethod.Post && request.RequestUri!.AbsolutePath == "/internal/runtime/execution/history/reset", "Reset used an unexpected endpoint.");
                Require(request.Headers.Authorization?.ToString() == "Bearer fixture-only-token", "Reset did not use the local credential.");
                Require(request.Content?.Headers.ContentType?.MediaType == "application/json", "Reset request was not JSON.");
                using var body = JsonDocument.Parse(await request.Content!.ReadAsStringAsync(token));
                Require(body.RootElement.GetProperty("confirm_permanent").GetBoolean() && body.RootElement.EnumerateObject().Count() == 1, "Reset supplied no confirmation or a caller-controlled path.");
                return new(HttpStatusCode.OK) { Content = new StringContent("{\"latest_seq\":9007199254740993,\"reclaimed_payload_bytes\":268435447,\"remaining_payload_bytes\":4096,\"protected_payload_bytes\":4096,\"cleanup_incomplete\":false}",Encoding.UTF8,"application/json") };
            }));
        var result = await client.ResetActivityHistoryAsync(CancellationToken.None);
        Require(result.LatestSeq == 9007199254740993 && result.ReclaimedPayloadBytes == 268435447 && result.ProtectedPayloadBytes == 4096 && !result.CleanupIncomplete, "Reset acknowledgement lost bytes, safety state or cursor precision.");
        using var cancelled = new CancellationTokenSource(); cancelled.Cancel();
        try { await client.ResetActivityHistoryAsync(cancelled.Token); throw new InvalidOperationException("Cancelled reset was dispatched."); }
        catch (OperationCanceledException) { }
        Require(requests == 1, "Reset retried a destructive request.");
        using var busy = new ActivityClient(_ => Task.FromResult(new ActivityConnection(new Uri("http://127.0.0.1:32123"),"fixture-only-token")),
            new ResetHandler((_,_) => Task.FromResult(new HttpResponseMessage(HttpStatusCode.Conflict) { Content = new StringContent("{\"error\":{\"code\":\"ACTIVITY_RESET_BUSY\",\"message\":\"busy\"}}") })));
        try { await busy.ResetActivityHistoryAsync(CancellationToken.None); throw new IOException("Busy reset was accepted."); }
        catch (InvalidOperationException ex) { Require(ex.Message == UiText.Get("ExecutionActivityResetBusy"), "Busy state was not localized."); }
        Console.WriteLine("Activity reset client contract passed: authenticated POST, explicit confirmation, precise acknowledgement, cancellation, no retry and localized busy state. No UI, network, installer or production runtime was started.");
    }

    private sealed class ResetHandler(Func<HttpRequestMessage,CancellationToken,Task<HttpResponseMessage>> respond) : HttpMessageHandler
    {
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken token) => respond(request,token);
    }
}
