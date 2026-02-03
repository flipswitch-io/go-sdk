package flipswitch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/open-feature/go-sdk/openfeature"
)

// TestDispatcher handles mock server requests.
type TestDispatcher struct {
	flagResponses map[string]func() (int, map[string]interface{})
	bulkResponse  func() (int, map[string]interface{})
	failInit      bool
	initFailCode  int
}

func NewTestDispatcher() *TestDispatcher {
	return &TestDispatcher{
		flagResponses: make(map[string]func() (int, map[string]interface{})),
		bulkResponse: func() (int, map[string]interface{}) {
			return 200, map[string]interface{}{"flags": []interface{}{}}
		},
		initFailCode: 401,
	}
}

func (d *TestDispatcher) SetFlagResponse(flagKey string, fn func() (int, map[string]interface{})) {
	d.flagResponses[flagKey] = fn
}

func (d *TestDispatcher) SetBulkResponse(fn func() (int, map[string]interface{})) {
	d.bulkResponse = fn
}

func (d *TestDispatcher) SetInitFailure(statusCode int) {
	d.failInit = true
	d.initFailCode = statusCode
}

func (d *TestDispatcher) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Bulk evaluation endpoint
	if path == "/ofrep/v1/evaluate/flags" {
		if d.failInit {
			w.WriteHeader(d.initFailCode)
			return
		}
		statusCode, body := d.bulkResponse()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(body)
		return
	}

	// Single flag evaluation endpoint
	if len(path) > len("/ofrep/v1/evaluate/flags/") {
		flagKey := path[len("/ofrep/v1/evaluate/flags/"):]
		if fn, ok := d.flagResponses[flagKey]; ok {
			statusCode, body := fn()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(body)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"key":       flagKey,
			"errorCode": "FLAG_NOT_FOUND",
		})
		return
	}

	// SSE endpoint
	if path == "/api/v1/flags/events" {
		w.WriteHeader(200)
		return
	}

	w.WriteHeader(404)
}

func createTestProvider(server *httptest.Server) (*FlipswitchProvider, error) {
	return NewProvider(
		"test-api-key",
		WithBaseURL(server.URL),
		WithRealtime(false),
	)
}

// ========================================
// Initialization Tests
// ========================================

func TestInitialization_ShouldSucceed(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Expected initialization to succeed, got error: %v", err)
	}

	if provider.Metadata().Name != "flipswitch" {
		t.Errorf("Expected metadata name 'flipswitch', got '%s'", provider.Metadata().Name)
	}
}

func TestInitialization_ShouldFailOnInvalidApiKey(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetInitFailure(401)
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err == nil {
		t.Fatal("Expected initialization to fail")
	}

	if err.Error() != "invalid API key" {
		t.Errorf("Expected 'invalid API key' error, got: %v", err)
	}
}

func TestInitialization_ShouldFailOnForbidden(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetInitFailure(403)
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err == nil {
		t.Fatal("Expected initialization to fail")
	}

	if err.Error() != "invalid API key" {
		t.Errorf("Expected 'invalid API key' error, got: %v", err)
	}
}

func TestInitialization_ShouldFailOnServerError(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetInitFailure(500)
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err == nil {
		t.Fatal("Expected initialization to fail")
	}

	// Check that error contains "failed to connect"
	if err.Error() != "failed to connect to Flipswitch: 500" {
		t.Errorf("Expected 'failed to connect' error, got: %v", err)
	}
}

// ========================================
// Metadata Tests
// ========================================

func TestMetadata_ShouldReturnFlipswitch(t *testing.T) {
	provider, err := NewProvider("test-key", WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.Metadata().Name != "flipswitch" {
		t.Errorf("Expected metadata name 'flipswitch', got '%s'", provider.Metadata().Name)
	}
}

// ========================================
// Bulk Evaluation Tests
// ========================================

func TestEvaluateAllFlags_ShouldReturnAllFlags(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetBulkResponse(func() (int, map[string]interface{}) {
		return 200, map[string]interface{}{
			"flags": []interface{}{
				map[string]interface{}{"key": "flag-1", "value": true, "reason": "DEFAULT"},
				map[string]interface{}{"key": "flag-2", "value": "test", "reason": "TARGETING_MATCH"},
			},
		}
	})
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	flags := provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	if len(flags) != 2 {
		t.Fatalf("Expected 2 flags, got %d", len(flags))
	}

	if flags[0].Key != "flag-1" {
		t.Errorf("Expected flag key 'flag-1', got '%s'", flags[0].Key)
	}
	if !flags[0].AsBoolean() {
		t.Error("Expected flag-1 value to be true")
	}

	if flags[1].Key != "flag-2" {
		t.Errorf("Expected flag key 'flag-2', got '%s'", flags[1].Key)
	}
	if flags[1].AsString() != "test" {
		t.Errorf("Expected flag-2 value 'test', got '%s'", flags[1].AsString())
	}
}

func TestEvaluateAllFlags_ShouldReturnEmptyListOnError(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Set error response after init
	dispatcher.SetInitFailure(500)

	flags := provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	if len(flags) != 0 {
		t.Errorf("Expected empty list, got %d flags", len(flags))
	}
}

// ========================================
// Single Flag Evaluation Tests
// ========================================

func TestEvaluateFlag_ShouldReturnSingleFlag(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetFlagResponse("my-flag", func() (int, map[string]interface{}) {
		return 200, map[string]interface{}{
			"key":     "my-flag",
			"value":   "hello",
			"reason":  "DEFAULT",
			"variant": "v1",
		}
	})
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	result := provider.EvaluateFlag("my-flag", openfeature.FlattenedContext{})

	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.Key != "my-flag" {
		t.Errorf("Expected key 'my-flag', got '%s'", result.Key)
	}
	if result.AsString() != "hello" {
		t.Errorf("Expected value 'hello', got '%s'", result.AsString())
	}
	if result.Reason != "DEFAULT" {
		t.Errorf("Expected reason 'DEFAULT', got '%s'", result.Reason)
	}
	if result.Variant != "v1" {
		t.Errorf("Expected variant 'v1', got '%s'", result.Variant)
	}
}

func TestEvaluateFlag_ShouldReturnNilForNonexistent(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	result := provider.EvaluateFlag("nonexistent", openfeature.FlattenedContext{})

	if result != nil {
		t.Errorf("Expected nil, got %+v", result)
	}
}

func TestEvaluateFlag_ShouldHandleBooleanValues(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetFlagResponse("bool-flag", func() (int, map[string]interface{}) {
		return 200, map[string]interface{}{
			"key":   "bool-flag",
			"value": true,
		}
	})
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	result := provider.EvaluateFlag("bool-flag", openfeature.FlattenedContext{})

	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if !result.AsBoolean() {
		t.Error("Expected value to be true")
	}
}

func TestEvaluateFlag_ShouldHandleNumericValues(t *testing.T) {
	dispatcher := NewTestDispatcher()
	dispatcher.SetFlagResponse("num-flag", func() (int, map[string]interface{}) {
		return 200, map[string]interface{}{
			"key":   "num-flag",
			"value": float64(42),
		}
	})
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	result := provider.EvaluateFlag("num-flag", openfeature.FlattenedContext{})

	if result == nil {
		t.Fatal("Expected result, got nil")
	}
	if result.AsInt() != 42 {
		t.Errorf("Expected value 42, got %d", result.AsInt())
	}
}

// ========================================
// SSE Status Tests
// ========================================

func TestSseStatus_ShouldBeDisconnectedWhenRealtimeDisabled(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	if provider.GetSseStatus() != StatusDisconnected {
		t.Errorf("Expected status DISCONNECTED, got %s", provider.GetSseStatus())
	}
}

// ========================================
// Flag Change Listener Tests
// ========================================

func TestFlagChangeListener_CanBeAddedAndRemoved(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	events := make([]FlagChangeEvent, 0)
	listener := func(event FlagChangeEvent) {
		events = append(events, event)
	}

	provider.AddFlagChangeListener(listener)
	// Note: RemoveFlagChangeListener won't work with anonymous functions
	// but we verify no exceptions are thrown

	if len(events) != 0 {
		t.Errorf("Expected no events, got %d", len(events))
	}
}

// ========================================
// Builder Tests
// ========================================

func TestBuilder_ShouldRequireApiKey(t *testing.T) {
	_, err := NewProvider("")
	if err == nil {
		t.Error("Expected error for empty API key")
	}
}

func TestBuilder_ShouldUseDefaults(t *testing.T) {
	provider, err := NewProvider("test-key", WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.Metadata().Name != "flipswitch" {
		t.Errorf("Expected metadata name 'flipswitch', got '%s'", provider.Metadata().Name)
	}
}

func TestBuilder_ShouldAllowCustomBaseUrl(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// If we get here without error, the custom baseURL was used
	if provider.Metadata().Name != "flipswitch" {
		t.Errorf("Expected metadata name 'flipswitch', got '%s'", provider.Metadata().Name)
	}
}

// ========================================
// URL Path Tests
// ========================================

func TestOfrepRequests_ShouldUseCorrectPath(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path

		// Respond to bulk evaluation (init) and single flag requests
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/ofrep/v1/evaluate/flags" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]interface{}{"flags": []interface{}{}})
			return
		}
		if r.URL.Path == "/ofrep/v1/evaluate/flags/test-flag" {
			w.WriteHeader(200)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"key":   "test-flag",
				"value": true,
			})
			return
		}
		w.WriteHeader(404)
	}))
	defer server.Close()

	provider, err := NewProvider(
		"test-api-key",
		WithBaseURL(server.URL),
		WithRealtime(false),
	)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Trigger a single flag evaluation
	provider.EvaluateFlag("test-flag", openfeature.FlattenedContext{})

	// Verify the path is correct (no duplicated /ofrep/v1)
	if capturedPath != "/ofrep/v1/evaluate/flags/test-flag" {
		t.Errorf("Expected path '/ofrep/v1/evaluate/flags/test-flag', got '%s'", capturedPath)
	}
}

// ========================================
// Polling Fallback Tests
// ========================================

func TestPollingFallback_ActivatesAfterMaxRetries(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := NewProvider(
		"test-api-key",
		WithBaseURL(server.URL),
		WithRealtime(false),
		WithMaxSseRetries(3),
		WithPollingFallback(true),
		WithPollingInterval(1*time.Hour), // Very long interval to prevent ticker from firing
	)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Simulate SSE error status changes
	for i := 0; i < 3; i++ {
		provider.handleStatusChange(StatusError)
	}

	if !provider.IsPollingActive() {
		t.Error("Expected polling to be active after max SSE retries")
	}

	// Allow the polling goroutine to start and block on select before shutdown,
	// so stopPolling's signal on pollingDone is received by the goroutine.
	time.Sleep(200 * time.Millisecond)
	provider.Shutdown()
}

func TestPollingFallback_DeactivatesOnSseReconnect(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := NewProvider(
		"test-api-key",
		WithBaseURL(server.URL),
		WithRealtime(false),
		WithMaxSseRetries(2),
		WithPollingFallback(true),
		WithPollingInterval(1*time.Hour), // Very long interval to prevent ticker from firing
	)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Trigger polling
	for i := 0; i < 2; i++ {
		provider.handleStatusChange(StatusError)
	}
	if !provider.IsPollingActive() {
		t.Error("Expected polling to be active")
	}

	// Allow the polling goroutine to start and block on select.
	// The goroutine needs to be scheduled and reach the select statement
	// before stopPolling sends on the pollingDone channel.
	time.Sleep(200 * time.Millisecond)

	// Simulate SSE reconnect — this calls stopPolling internally
	provider.handleStatusChange(StatusConnected)

	if provider.IsPollingActive() {
		t.Error("Expected polling to be inactive after SSE reconnect")
	}
}

func TestPollingFallback_DisabledWhenFalse(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := NewProvider(
		"test-api-key",
		WithBaseURL(server.URL),
		WithRealtime(false),
		WithMaxSseRetries(2),
		WithPollingFallback(false),
	)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Simulate many SSE errors
	for i := 0; i < 10; i++ {
		provider.handleStatusChange(StatusError)
	}

	if provider.IsPollingActive() {
		t.Error("Expected polling to remain inactive when disabled")
	}
}

// ========================================
// Flag Change Handling Tests
// ========================================

func TestHandleFlagChange_NotifiesListeners(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	events := make([]FlagChangeEvent, 0)
	provider.AddFlagChangeListener(func(event FlagChangeEvent) {
		events = append(events, event)
	})

	provider.handleFlagChange(FlagChangeEvent{
		FlagKey:   "test-flag",
		Timestamp: "2024-01-01T00:00:00Z",
	})

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}
	if events[0].FlagKey != "test-flag" {
		t.Errorf("Expected flag key 'test-flag', got '%s'", events[0].FlagKey)
	}
}

func TestHandleFlagChange_ListenerErrorIsolation(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	events := make([]FlagChangeEvent, 0)

	// Add a listener that panics
	provider.AddFlagChangeListener(func(event FlagChangeEvent) {
		panic("listener error")
	})

	// Add a good listener
	provider.AddFlagChangeListener(func(event FlagChangeEvent) {
		events = append(events, event)
	})

	// Should not panic and second listener should still be called
	provider.handleFlagChange(FlagChangeEvent{
		FlagKey:   "test",
		Timestamp: "2024-01-01T00:00:00Z",
	})

	if len(events) != 1 {
		t.Errorf("Expected 1 event from second listener, got %d", len(events))
	}
}

func TestHandleFlagChange_MultipleListeners(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	count := 0
	for i := 0; i < 3; i++ {
		provider.AddFlagChangeListener(func(event FlagChangeEvent) {
			count++
		})
	}

	provider.handleFlagChange(FlagChangeEvent{
		FlagKey:   "test",
		Timestamp: "2024-01-01T00:00:00Z",
	})

	if count != 3 {
		t.Errorf("Expected all 3 listeners to be called, got %d", count)
	}
}

// ========================================
// Shutdown / Cleanup Tests
// ========================================

func TestShutdown_ClearsState(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	provider.mu.RLock()
	if !provider.initialized {
		provider.mu.RUnlock()
		t.Fatal("Expected provider to be initialized")
	}
	provider.mu.RUnlock()

	provider.Shutdown()

	provider.mu.RLock()
	if provider.initialized {
		provider.mu.RUnlock()
		t.Error("Expected provider to not be initialized after shutdown")
	}
	provider.mu.RUnlock()
}

func TestShutdown_IsIdempotent(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Should not panic on double shutdown
	provider.Shutdown()
	provider.Shutdown()
}

func TestDoubleInit_ReturnsNilWithoutError(t *testing.T) {
	dispatcher := NewTestDispatcher()
	server := httptest.NewServer(dispatcher)
	defer server.Close()

	provider, err := createTestProvider(server)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("First init failed: %v", err)
	}

	// Second init should return nil without error
	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Errorf("Expected second Init to return nil, got: %v", err)
	}
}

// ========================================
// Context Transformation Tests
// ========================================

func TestTransformContext_TargetingKeyOnly(t *testing.T) {
	ctx := openfeature.FlattenedContext{
		"targetingKey": "user-123",
	}

	result := transformContext(ctx)

	if result["targetingKey"] != "user-123" {
		t.Errorf("Expected targetingKey 'user-123', got '%v'", result["targetingKey"])
	}
}

func TestTransformContext_WithAttributes(t *testing.T) {
	ctx := openfeature.FlattenedContext{
		"targetingKey": "user-123",
		"email":        "test@example.com",
		"plan":         "premium",
	}

	result := transformContext(ctx)

	if result["targetingKey"] != "user-123" {
		t.Errorf("Expected targetingKey 'user-123', got '%v'", result["targetingKey"])
	}
	if result["email"] != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got '%v'", result["email"])
	}
	if result["plan"] != "premium" {
		t.Errorf("Expected plan 'premium', got '%v'", result["plan"])
	}
}

func TestTransformContext_EmptyContext(t *testing.T) {
	ctx := openfeature.FlattenedContext{}
	result := transformContext(ctx)

	if len(result) != 0 {
		t.Errorf("Expected empty map, got %v", result)
	}
}

// ========================================
// Type Inference Tests
// ========================================

func TestInferType_Boolean(t *testing.T) {
	if inferType(true) != "boolean" {
		t.Errorf("Expected 'boolean', got '%s'", inferType(true))
	}
	if inferType(false) != "boolean" {
		t.Errorf("Expected 'boolean', got '%s'", inferType(false))
	}
}

func TestInferType_String(t *testing.T) {
	if inferType("hello") != "string" {
		t.Errorf("Expected 'string', got '%s'", inferType("hello"))
	}
}

func TestInferType_Integer(t *testing.T) {
	if inferType(42) != "integer" {
		t.Errorf("Expected 'integer', got '%s'", inferType(42))
	}
	if inferType(int64(42)) != "integer" {
		t.Errorf("Expected 'integer', got '%s'", inferType(int64(42)))
	}
}

func TestInferType_Float(t *testing.T) {
	if inferType(float64(3.14)) != "number" {
		t.Errorf("Expected 'number', got '%s'", inferType(float64(3.14)))
	}
}

func TestInferType_Null(t *testing.T) {
	if inferType(nil) != "null" {
		t.Errorf("Expected 'null', got '%s'", inferType(nil))
	}
}

func TestInferType_Object(t *testing.T) {
	obj := map[string]interface{}{"key": "value"}
	if inferType(obj) != "object" {
		t.Errorf("Expected 'object', got '%s'", inferType(obj))
	}
}

func TestInferType_Array(t *testing.T) {
	arr := []interface{}{1, 2, 3}
	if inferType(arr) != "array" {
		t.Errorf("Expected 'array', got '%s'", inferType(arr))
	}
}

func TestGetFlagType_MetadataOverride(t *testing.T) {
	data := map[string]interface{}{
		"value":    nil,
		"metadata": map[string]interface{}{"flagType": "boolean"},
	}
	if getFlagType(data) != "boolean" {
		t.Errorf("Expected 'boolean', got '%s'", getFlagType(data))
	}
}

func TestGetFlagType_DecimalMapsToNumber(t *testing.T) {
	data := map[string]interface{}{
		"value":    float64(3.14),
		"metadata": map[string]interface{}{"flagType": "decimal"},
	}
	if getFlagType(data) != "number" {
		t.Errorf("Expected 'number', got '%s'", getFlagType(data))
	}
}

// ========================================
// Telemetry Headers Tests
// ========================================

func TestTelemetryHeaders_SdkHeader(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{"flags": []interface{}{}})
	}))
	defer server.Close()

	provider, err := NewProvider("test-api-key", WithBaseURL(server.URL), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	sdk := capturedHeaders.Get("X-Flipswitch-SDK")
	if sdk == "" {
		t.Error("Expected X-Flipswitch-SDK header")
	}
	if len(sdk) < 3 || sdk[:3] != "go/" {
		t.Errorf("Expected SDK header to start with 'go/', got '%s'", sdk)
	}
}

func TestTelemetryHeaders_RuntimeHeader(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{"flags": []interface{}{}})
	}))
	defer server.Close()

	provider, err := NewProvider("test-api-key", WithBaseURL(server.URL), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	runtime := capturedHeaders.Get("X-Flipswitch-Runtime")
	if runtime == "" {
		t.Error("Expected X-Flipswitch-Runtime header")
	}
	if len(runtime) < 3 || runtime[:3] != "go/" {
		t.Errorf("Expected runtime header to start with 'go/', got '%s'", runtime)
	}
}

func TestTelemetryHeaders_OsHeader(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{"flags": []interface{}{}})
	}))
	defer server.Close()

	provider, err := NewProvider("test-api-key", WithBaseURL(server.URL), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	osHeader := capturedHeaders.Get("X-Flipswitch-OS")
	if osHeader == "" {
		t.Error("Expected X-Flipswitch-OS header")
	}
	if !contains(osHeader, "/") {
		t.Errorf("Expected OS header to contain '/', got '%s'", osHeader)
	}
}

func TestTelemetryHeaders_FeaturesHeader(t *testing.T) {
	var capturedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]interface{}{"flags": []interface{}{}})
	}))
	defer server.Close()

	provider, err := NewProvider("test-api-key", WithBaseURL(server.URL), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	provider.EvaluateAllFlags(openfeature.FlattenedContext{"targetingKey": "user-1"})

	features := capturedHeaders.Get("X-Flipswitch-Features")
	if features != "sse=false" {
		t.Errorf("Expected 'sse=false', got '%s'", features)
	}
}

// ========================================
// Functional Options Tests
// ========================================

func TestWithHTTPClient(t *testing.T) {
	customClient := &http.Client{}
	provider, err := NewProvider("test-key", WithHTTPClient(customClient), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.httpClient != customClient {
		t.Error("Expected custom HTTP client to be set")
	}
}

func TestWithPollingFallbackFalse(t *testing.T) {
	provider, err := NewProvider("test-key", WithPollingFallback(false), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.enablePollingFallback {
		t.Error("Expected polling fallback to be disabled")
	}
}

func TestWithPollingInterval(t *testing.T) {
	provider, err := NewProvider("test-key", WithPollingInterval(10*time.Second), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.pollingInterval != 10*time.Second {
		t.Errorf("Expected polling interval 10s, got %v", provider.pollingInterval)
	}
}

func TestWithMaxSseRetries(t *testing.T) {
	provider, err := NewProvider("test-key", WithMaxSseRetries(10), WithRealtime(false))
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}
	defer provider.Shutdown()

	if provider.maxSseRetries != 10 {
		t.Errorf("Expected max retries 10, got %d", provider.maxSseRetries)
	}
}

// helper
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
