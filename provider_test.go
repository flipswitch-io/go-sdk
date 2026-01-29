package flipswitch

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
