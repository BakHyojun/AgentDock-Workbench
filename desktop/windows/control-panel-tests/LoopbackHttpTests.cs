using System.Net;
using System.Net.Sockets;
using System.Text;
using AgentDock.ControlPanel;

internal static class LoopbackHttpTests
{
    private sealed class RecordingProxy(Uri address) : IWebProxy
    {
        public ICredentials? Credentials { get; set; }
        public Uri GetProxy(Uri destination) => address;
        public bool IsBypassed(Uri host) => false;
    }

    // A system proxy that does not bypass loopback would receive the Core's
    // Bearer token, and a redirect could forward it off loopback.
    internal static async Task Run(Action<bool, string> check)
    {
        var origin = new TcpListener(IPAddress.Loopback, 0);
        var proxy = new TcpListener(IPAddress.Loopback, 0);
        origin.Start();
        proxy.Start();
        var previous = HttpClient.DefaultProxy;
        try
        {
            HttpClient.DefaultProxy = new RecordingProxy(new Uri($"http://127.0.0.1:{((IPEndPoint)proxy.LocalEndpoint).Port}"));
            using var http = new HttpClient(LoopbackHttp.CreateHandler()) { Timeout = TimeSpan.FromSeconds(5) };
            var originAccept = origin.AcceptTcpClientAsync();
            var proxyAccept = proxy.AcceptTcpClientAsync();
            var request = http.GetAsync($"http://127.0.0.1:{((IPEndPoint)origin.LocalEndpoint).Port}/healthz");
            var first = await Task.WhenAny(originAccept, proxyAccept, Task.Delay(TimeSpan.FromSeconds(5)));
            check(first == originAccept, "loopback Core request ignores the system proxy");
            using (var client = await originAccept)
            using (var stream = client.GetStream())
            {
                _ = await stream.ReadAsync(new byte[4096]);
                await stream.WriteAsync(Encoding.ASCII.GetBytes("HTTP/1.1 302 Found\r\nLocation: http://127.0.0.2:9/elsewhere\r\nContent-Length: 0\r\nConnection: close\r\n\r\n"));
            }
            using var response = await request;
            check(response.StatusCode == HttpStatusCode.Found, "loopback Core request does not follow redirects");
        }
        finally
        {
            HttpClient.DefaultProxy = previous;
            origin.Stop();
            proxy.Stop();
        }
    }
}
