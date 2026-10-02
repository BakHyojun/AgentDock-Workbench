package httpx

import (
	"net/http"
	"strings"
	"testing"
)

func TestActivityResetHTTPAuthenticationConfirmationAndScope(t *testing.T) {
	f := newExecutionHTTPFixture(t)
	call := f.call(t, "reset-http", "agentdock_context", map[string]any{})
	path := "/internal/runtime/execution/history/reset"
	for _, test := range []struct {
		method string
		body   any
		want   int
	}{
		{"GET", nil, 405}, {"POST", map[string]any{}, 400},
		{"POST", map[string]any{"confirm_permanent": true, "path": "../"}, 400},
	} {
		_, status := f.request(t, test.method, path, test.body)
		if status != test.want {
			t.Fatal("reset ingress accepted invalid request", status, test.want)
		}
	}
	request, err := http.NewRequest("POST", f.server.URL+path, strings.NewReader(`{"confirm_permanent":true}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatal("reset accepted anonymous request", response.StatusCode)
	}
	_, status := f.request(t, "GET", "/internal/runtime/calls/"+call["call_id"].(string), nil)
	if status != 200 {
		t.Fatal("rejected reset lost record", status)
	}
	result, status := f.request(t, "POST", path, map[string]any{"confirm_permanent": true})
	if status != 200 || result["latest_seq"].(float64) <= 0 {
		t.Fatal("confirmed reset failed", result, status)
	}
	_, status = f.request(t, "GET", "/internal/runtime/calls/"+call["call_id"].(string), nil)
	if status != 404 {
		t.Fatal("old detail remained available", status)
	}
	_, status = f.request(t, "GET", "/internal/runtime/conversations/"+call["conversation_id"].(string), nil)
	if status != 200 {
		t.Fatal("reset deleted conversation registry", status)
	}
	f.call(t, "reset-http", "agentdock_context", map[string]any{})
}
