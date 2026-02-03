package flipswitch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Unit Tests
// ---------------------------------------------------------------------------

func TestSseClient_InitialStatus(t *testing.T) {
	t.Parallel()

	client := NewSseClient("http://localhost", "test-key", nil, nil, nil)
	defer client.Close()

	if got := client.GetStatus(); got != StatusDisconnected {
		t.Errorf("expected initial status %q, got %q", StatusDisconnected, got)
	}
}

func TestSseClient_Close(t *testing.T) {
	t.Parallel()

	client := NewSseClient("http://localhost", "test-key", nil, nil, nil)
	client.Close()

	if got := client.GetStatus(); got != StatusDisconnected {
		t.Errorf("expected status %q after Close, got %q", StatusDisconnected, got)
	}

	client.mu.RLock()
	closed := client.closed
	client.mu.RUnlock()

	if !closed {
		t.Error("expected closed to be true after Close")
	}
}

func TestSseClient_ClosePreventReconnect(t *testing.T) {
	t.Parallel()

	client := NewSseClient("http://localhost", "test-key", nil, nil, nil)
	client.Close()

	// Connect should be a no-op on a closed client.
	client.Connect()

	// Give it a moment; status should remain disconnected.
	time.Sleep(50 * time.Millisecond)

	if got := client.GetStatus(); got != StatusDisconnected {
		t.Errorf("expected status %q after Connect on closed client, got %q", StatusDisconnected, got)
	}
}

func TestSseClient_HandleEvent_FlagUpdated(t *testing.T) {
	t.Parallel()

	received := make(chan FlagChangeEvent, 1)
	client := NewSseClient("http://localhost", "test-key", nil,
		func(event FlagChangeEvent) {
			received <- event
		}, nil)
	defer client.Close()

	client.handleEvent("flag-updated", `{"flagKey":"my-flag","timestamp":"2024-01-01T00:00:00Z"}`)

	select {
	case event := <-received:
		if event.FlagKey != "my-flag" {
			t.Errorf("expected FlagKey %q, got %q", "my-flag", event.FlagKey)
		}
		if event.Timestamp != "2024-01-01T00:00:00Z" {
			t.Errorf("expected Timestamp %q, got %q", "2024-01-01T00:00:00Z", event.Timestamp)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flag change event")
	}
}

func TestSseClient_HandleEvent_ConfigUpdated(t *testing.T) {
	t.Parallel()

	received := make(chan FlagChangeEvent, 1)
	client := NewSseClient("http://localhost", "test-key", nil,
		func(event FlagChangeEvent) {
			received <- event
		}, nil)
	defer client.Close()

	client.handleEvent("config-updated", `{"timestamp":"2024-06-15T12:00:00Z"}`)

	select {
	case event := <-received:
		if event.FlagKey != "" {
			t.Errorf("expected empty FlagKey for config-updated, got %q", event.FlagKey)
		}
		if event.Timestamp != "2024-06-15T12:00:00Z" {
			t.Errorf("expected Timestamp %q, got %q", "2024-06-15T12:00:00Z", event.Timestamp)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for config-updated event")
	}
}

func TestSseClient_HandleEvent_ApiKeyRotated(t *testing.T) {
	t.Parallel()

	called := false
	client := NewSseClient("http://localhost", "test-key", nil,
		func(event FlagChangeEvent) {
			called = true
		}, nil)
	defer client.Close()

	client.handleEvent("api-key-rotated", `{"validUntil":"2024-12-01T00:00:00Z","timestamp":"2024-01-01T00:00:00Z"}`)

	if called {
		t.Error("expected onFlagChange NOT to be called for api-key-rotated")
	}
}

func TestSseClient_HandleEvent_Heartbeat(t *testing.T) {
	t.Parallel()

	called := false
	client := NewSseClient("http://localhost", "test-key", nil,
		func(event FlagChangeEvent) {
			called = true
		}, nil)
	defer client.Close()

	client.handleEvent("heartbeat", "")

	if called {
		t.Error("expected onFlagChange NOT to be called for heartbeat")
	}
}

func TestSseClient_HandleEvent_MalformedJson(t *testing.T) {
	t.Parallel()

	called := false
	client := NewSseClient("http://localhost", "test-key", nil,
		func(event FlagChangeEvent) {
			called = true
		}, nil)
	defer client.Close()

	// Should not panic or invoke the callback.
	client.handleEvent("flag-updated", `{not valid json}`)
	client.handleEvent("config-updated", `{not valid json}`)
	client.handleEvent("api-key-rotated", `{not valid json}`)

	if called {
		t.Error("expected onFlagChange NOT to be called for malformed JSON")
	}
}

func TestSseClient_StatusChangeCallback(t *testing.T) {
	t.Parallel()

	statuses := make(chan ConnectionStatus, 10)
	client := NewSseClient("http://localhost", "test-key", nil, nil,
		func(status ConnectionStatus) {
			statuses <- status
		})
	defer client.Close()

	client.updateStatus(StatusConnecting)
	client.updateStatus(StatusConnected)
	client.updateStatus(StatusError)
	client.updateStatus(StatusDisconnected)

	expected := []ConnectionStatus{StatusConnecting, StatusConnected, StatusError, StatusDisconnected}
	for _, want := range expected {
		select {
		case got := <-statuses:
			if got != want {
				t.Errorf("expected status %q, got %q", want, got)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for status %q", want)
		}
	}
}

func TestSseClient_ExponentialBackoff(t *testing.T) {
	t.Parallel()

	client := NewSseClient("http://localhost", "test-key", nil, nil, nil)
	defer client.Close()

	// Initial delay should be minRetryDelay (1s).
	client.mu.RLock()
	if client.retryDelay != minRetryDelay {
		t.Errorf("expected initial retryDelay %v, got %v", minRetryDelay, client.retryDelay)
	}
	client.mu.RUnlock()

	// Simulate the backoff doubling that scheduleReconnect performs,
	// but without actually waiting. We directly manipulate the delay
	// the same way scheduleReconnect does after the wait.
	expectedDelays := []time.Duration{
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped at maxRetryDelay
		30 * time.Second, // stays at max
	}

	for i, want := range expectedDelays {
		client.mu.Lock()
		if client.retryDelay < maxRetryDelay {
			client.retryDelay = client.retryDelay * 2
			if client.retryDelay > maxRetryDelay {
				client.retryDelay = maxRetryDelay
			}
		}
		got := client.retryDelay
		client.mu.Unlock()

		if got != want {
			t.Errorf("step %d: expected retryDelay %v, got %v", i, want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Integration Tests
// ---------------------------------------------------------------------------

// sseFrame formats an SSE frame with an event type and JSON data.
func sseFrame(eventType, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", eventType, data)
}

func TestSseClient_Integration_Connection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flags/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		// Keep connection open until the client disconnects.
		<-r.Context().Done()
	}))
	defer server.Close()

	statusCh := make(chan ConnectionStatus, 10)
	client := NewSseClient(server.URL, "test-key", nil, nil,
		func(status ConnectionStatus) {
			statusCh <- status
		})
	defer client.Close()

	client.Connect()

	// Wait for "connecting" then "connected".
	deadline := time.After(5 * time.Second)
	gotConnected := false
	for !gotConnected {
		select {
		case s := <-statusCh:
			if s == StatusConnected {
				gotConnected = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for connected status")
		}
	}

	if got := client.GetStatus(); got != StatusConnected {
		t.Errorf("expected status %q, got %q", StatusConnected, got)
	}
}

func TestSseClient_Integration_FlagUpdatedEvent(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flags/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		flusher.Flush()

		// Wait until the test signals that the client is connected.
		<-ready

		frame := sseFrame("flag-updated", `{"flagKey":"beta-feature","timestamp":"2024-03-15T10:30:00Z"}`)
		fmt.Fprint(w, frame)
		flusher.Flush()

		// Keep connection open a bit so the client can process the event.
		<-r.Context().Done()
	}))
	defer server.Close()

	flagCh := make(chan FlagChangeEvent, 1)
	statusCh := make(chan ConnectionStatus, 10)

	client := NewSseClient(server.URL, "test-key", nil,
		func(event FlagChangeEvent) {
			flagCh <- event
		},
		func(status ConnectionStatus) {
			statusCh <- status
		})
	defer client.Close()

	client.Connect()

	// Wait until connected, then signal the server to send the event.
	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-statusCh:
			if s == StatusConnected {
				close(ready)
				goto waitForEvent
			}
		case <-deadline:
			t.Fatal("timed out waiting for connected status")
		}
	}

waitForEvent:
	select {
	case event := <-flagCh:
		if event.FlagKey != "beta-feature" {
			t.Errorf("expected FlagKey %q, got %q", "beta-feature", event.FlagKey)
		}
		if event.Timestamp != "2024-03-15T10:30:00Z" {
			t.Errorf("expected Timestamp %q, got %q", "2024-03-15T10:30:00Z", event.Timestamp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for flag-updated event")
	}
}

func TestSseClient_Integration_ConfigUpdatedEvent(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flags/events" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		flusher.Flush()

		<-ready

		frame := sseFrame("config-updated", `{"timestamp":"2024-05-20T08:00:00Z"}`)
		fmt.Fprint(w, frame)
		flusher.Flush()

		<-r.Context().Done()
	}))
	defer server.Close()

	flagCh := make(chan FlagChangeEvent, 1)
	statusCh := make(chan ConnectionStatus, 10)

	client := NewSseClient(server.URL, "test-key", nil,
		func(event FlagChangeEvent) {
			flagCh <- event
		},
		func(status ConnectionStatus) {
			statusCh <- status
		})
	defer client.Close()

	client.Connect()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case s := <-statusCh:
			if s == StatusConnected {
				close(ready)
				goto waitForEvent
			}
		case <-deadline:
			t.Fatal("timed out waiting for connected status")
		}
	}

waitForEvent:
	select {
	case event := <-flagCh:
		if event.FlagKey != "" {
			t.Errorf("expected empty FlagKey for config-updated, got %q", event.FlagKey)
		}
		if event.Timestamp != "2024-05-20T08:00:00Z" {
			t.Errorf("expected Timestamp %q, got %q", "2024-05-20T08:00:00Z", event.Timestamp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for config-updated event")
	}
}

func TestSseClient_Integration_Reconnection(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		connections int
	)
	connCh := make(chan int, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/flags/events" {
			http.NotFound(w, r)
			return
		}

		mu.Lock()
		connections++
		connNum := connections
		mu.Unlock()
		connCh <- connNum

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		flusher.Flush()

		if connNum == 1 {
			// Close the first connection immediately to trigger a reconnect.
			return
		}

		// Keep the second connection alive until the client disconnects.
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewSseClient(server.URL, "test-key", nil, nil, nil)
	// Use a short retry delay so the test does not take too long.
	client.mu.Lock()
	client.retryDelay = 50 * time.Millisecond
	client.mu.Unlock()
	defer client.Close()

	client.Connect()

	// Wait for at least two connections.
	deadline := time.After(10 * time.Second)
	seen := 0
	for seen < 2 {
		select {
		case <-connCh:
			seen++
		case <-deadline:
			t.Fatalf("timed out waiting for reconnection; saw %d connections", seen)
		}
	}

	if seen < 2 {
		t.Errorf("expected at least 2 connections, got %d", seen)
	}
}

func TestSseClient_Integration_ErrorOnNon200(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	statusCh := make(chan ConnectionStatus, 10)
	client := NewSseClient(server.URL, "test-key", nil, nil,
		func(status ConnectionStatus) {
			statusCh <- status
		})
	// Use a very long retry so we only see the first attempt.
	client.mu.Lock()
	client.retryDelay = 10 * time.Second
	client.mu.Unlock()
	defer client.Close()

	client.Connect()

	// Collect statuses until we see StatusError.
	deadline := time.After(5 * time.Second)
	gotError := false
	for !gotError {
		select {
		case s := <-statusCh:
			if s == StatusError {
				gotError = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for error status")
		}
	}

	if got := client.GetStatus(); got != StatusError {
		t.Errorf("expected status %q, got %q", StatusError, got)
	}
}
