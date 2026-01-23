// Package flipswitch provides an OpenFeature provider for Flipswitch with
// real-time SSE support.
//
// This package wraps the OFREP provider for flag evaluation and adds
// real-time updates via Server-Sent Events (SSE).
//
// Example:
//
//	provider, err := flipswitch.NewProvider("your-api-key")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer provider.Shutdown()
//
//	openfeature.SetProvider(provider)
//	client := openfeature.NewClient("my-app")
//
//	darkMode, _ := client.BooleanValue(ctx, "dark-mode", false, nil)
package flipswitch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/open-feature/go-sdk-contrib/providers/ofrep"
	"github.com/open-feature/go-sdk/openfeature"
)

const defaultBaseURL = "https://api.flipswitch.io"

// FlipswitchProvider is an OpenFeature provider for Flipswitch with
// real-time SSE support.
type FlipswitchProvider struct {
	baseURL        string
	apiKey         string
	enableRealtime bool
	httpClient     *http.Client

	ofrepProvider       *ofrep.Provider
	flagChangeListeners []FlagChangeHandler
	sseClient           *SseClient
	initialized         bool
	mu                  sync.RWMutex
}

// NewProvider creates a new FlipswitchProvider with the given API key.
// Returns an error if the API key is empty.
func NewProvider(apiKey string, opts ...Option) (*FlipswitchProvider, error) {
	if apiKey == "" {
		return nil, errors.New("apiKey is required")
	}

	p := &FlipswitchProvider{
		baseURL:             defaultBaseURL,
		apiKey:              apiKey,
		enableRealtime:      true,
		httpClient:          &http.Client{},
		flagChangeListeners: make([]FlagChangeHandler, 0),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.baseURL = strings.TrimSuffix(p.baseURL, "/")

	// Create underlying OFREP provider for flag evaluation
	p.ofrepProvider = ofrep.NewProvider(
		p.baseURL+"/ofrep/v1",
		ofrep.WithHeader("X-API-Key", p.apiKey),
	)

	return p, nil
}

// Option is a functional option for configuring the provider.
type Option func(*FlipswitchProvider)

// WithBaseURL sets the Flipswitch server base URL.
func WithBaseURL(url string) Option {
	return func(p *FlipswitchProvider) {
		p.baseURL = url
	}
}

// WithRealtime enables or disables real-time SSE updates.
func WithRealtime(enabled bool) Option {
	return func(p *FlipswitchProvider) {
		p.enableRealtime = enabled
	}
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(p *FlipswitchProvider) {
		p.httpClient = client
	}
}

// Metadata returns the provider metadata.
func (p *FlipswitchProvider) Metadata() openfeature.Metadata {
	return openfeature.Metadata{
		Name: "flipswitch",
	}
}

// Init initializes the provider. Validates the API key and starts SSE connection
// if real-time is enabled.
func (p *FlipswitchProvider) Init(evaluationContext openfeature.EvaluationContext) error {
	// Validate API key first (OFREP provider doesn't throw on auth errors during init)
	if err := p.validateAPIKey(); err != nil {
		return err
	}

	// Start SSE connection for real-time updates
	if p.enableRealtime {
		p.startSseConnection()
	}

	p.mu.Lock()
	p.initialized = true
	p.mu.Unlock()

	log.Printf("[Flipswitch] Provider initialized (realtime=%v)", p.enableRealtime)
	return nil
}

func (p *FlipswitchProvider) validateAPIKey() error {
	url := p.baseURL + "/ofrep/v1/evaluate/flags"

	body := map[string]interface{}{
		"context": map[string]string{
			"targetingKey": "_init_",
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to connect to Flipswitch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return errors.New("invalid API key")
	}

	if resp.StatusCode >= 500 {
		return fmt.Errorf("failed to connect to Flipswitch: %d", resp.StatusCode)
	}

	return nil
}

// Shutdown shuts down the provider and closes all connections.
func (p *FlipswitchProvider) Shutdown() {
	if p.sseClient != nil {
		p.sseClient.Close()
		p.sseClient = nil
	}

	p.mu.Lock()
	p.initialized = false
	p.mu.Unlock()

	log.Println("[Flipswitch] Provider shut down")
}

func (p *FlipswitchProvider) startSseConnection() {
	p.sseClient = NewSseClient(
		p.baseURL,
		p.apiKey,
		p.handleFlagChange,
		p.handleStatusChange,
	)
	p.sseClient.Connect()
}

func (p *FlipswitchProvider) handleFlagChange(event FlagChangeEvent) {
	p.mu.RLock()
	listeners := make([]FlagChangeHandler, len(p.flagChangeListeners))
	copy(listeners, p.flagChangeListeners)
	p.mu.RUnlock()

	for _, listener := range listeners {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[Flipswitch] Error in flag change listener: %v", r)
				}
			}()
			listener(event)
		}()
	}
}

func (p *FlipswitchProvider) handleStatusChange(status ConnectionStatus) {
	if status == StatusError {
		log.Println("[Flipswitch] SSE connection error, provider is stale")
	} else if status == StatusConnected {
		log.Println("[Flipswitch] SSE connection restored")
	}
}

// AddFlagChangeListener adds a listener for flag change events.
func (p *FlipswitchProvider) AddFlagChangeListener(handler FlagChangeHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flagChangeListeners = append(p.flagChangeListeners, handler)
}

// RemoveFlagChangeListener removes a flag change listener.
func (p *FlipswitchProvider) RemoveFlagChangeListener(handler FlagChangeHandler) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, h := range p.flagChangeListeners {
		// Compare function pointers - this may not work for all cases
		if &h == &handler {
			p.flagChangeListeners = append(p.flagChangeListeners[:i], p.flagChangeListeners[i+1:]...)
			return
		}
	}
}

// GetSseStatus returns the current SSE connection status.
func (p *FlipswitchProvider) GetSseStatus() ConnectionStatus {
	if p.sseClient != nil {
		return p.sseClient.GetStatus()
	}
	return StatusDisconnected
}

// ReconnectSse forces a reconnection of the SSE client.
func (p *FlipswitchProvider) ReconnectSse() {
	if p.enableRealtime && p.sseClient != nil {
		p.sseClient.Close()
		p.startSseConnection()
	}
}

// ===============================
// Flag Resolution Methods - Delegated to OFREP Provider
// ===============================

// Hooks returns any hooks the provider implements.
func (p *FlipswitchProvider) Hooks() []openfeature.Hook {
	return p.ofrepProvider.Hooks()
}

// BooleanEvaluation evaluates a boolean flag.
func (p *FlipswitchProvider) BooleanEvaluation(
	ctx context.Context,
	flag string,
	defaultValue bool,
	evalCtx openfeature.FlattenedContext,
) openfeature.BoolResolutionDetail {
	return p.ofrepProvider.BooleanEvaluation(ctx, flag, defaultValue, evalCtx)
}

// StringEvaluation evaluates a string flag.
func (p *FlipswitchProvider) StringEvaluation(
	ctx context.Context,
	flag string,
	defaultValue string,
	evalCtx openfeature.FlattenedContext,
) openfeature.StringResolutionDetail {
	return p.ofrepProvider.StringEvaluation(ctx, flag, defaultValue, evalCtx)
}

// FloatEvaluation evaluates a float flag.
func (p *FlipswitchProvider) FloatEvaluation(
	ctx context.Context,
	flag string,
	defaultValue float64,
	evalCtx openfeature.FlattenedContext,
) openfeature.FloatResolutionDetail {
	return p.ofrepProvider.FloatEvaluation(ctx, flag, defaultValue, evalCtx)
}

// IntEvaluation evaluates an integer flag.
func (p *FlipswitchProvider) IntEvaluation(
	ctx context.Context,
	flag string,
	defaultValue int64,
	evalCtx openfeature.FlattenedContext,
) openfeature.IntResolutionDetail {
	return p.ofrepProvider.IntEvaluation(ctx, flag, defaultValue, evalCtx)
}

// ObjectEvaluation evaluates an object flag.
func (p *FlipswitchProvider) ObjectEvaluation(
	ctx context.Context,
	flag string,
	defaultValue interface{},
	evalCtx openfeature.FlattenedContext,
) openfeature.InterfaceResolutionDetail {
	return p.ofrepProvider.ObjectEvaluation(ctx, flag, defaultValue, evalCtx)
}

// ===============================
// Bulk Flag Evaluation (Direct HTTP - OFREP providers don't expose bulk API)
// ===============================

func transformContext(evalCtx openfeature.FlattenedContext) map[string]interface{} {
	result := make(map[string]interface{})

	for k, v := range evalCtx {
		result[k] = v
	}

	return result
}

func inferType(value interface{}) string {
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case bool:
		return "boolean"
	case int, int64:
		return "integer"
	case float64:
		return "number"
	case string:
		return "string"
	case []interface{}:
		return "array"
	case map[string]interface{}:
		return "object"
	default:
		return "unknown"
	}
}

func getFlagType(data map[string]interface{}) string {
	if metadata, ok := data["metadata"].(map[string]interface{}); ok {
		if flagType, ok := metadata["flagType"].(string); ok {
			switch flagType {
			case "boolean":
				return "boolean"
			case "string":
				return "string"
			case "integer":
				return "integer"
			case "decimal":
				return "number"
			}
		}
	}
	return inferType(data["value"])
}

func getString(data map[string]interface{}, key, defaultValue string) string {
	if v, ok := data[key].(string); ok {
		return v
	}
	return defaultValue
}

func isSuccess(statusCode int) bool {
	return statusCode >= 200 && statusCode < 300
}

// EvaluateAllFlags evaluates all flags for the given context.
// Returns a list of all flag evaluations with their keys, values, types, and reasons.
//
// Note: This method makes direct HTTP calls since OFREP providers don't expose
// the bulk evaluation API.
func (p *FlipswitchProvider) EvaluateAllFlags(evalCtx openfeature.FlattenedContext) []FlagEvaluation {
	results := make([]FlagEvaluation, 0)

	url := p.baseURL + "/ofrep/v1/evaluate/flags"

	body := map[string]interface{}{
		"context": transformContext(evalCtx),
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("[Flipswitch] Error evaluating all flags: %v", err)
		return results
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[Flipswitch] Error evaluating all flags: %v", err)
		return results
	}
	defer resp.Body.Close()

	if !isSuccess(resp.StatusCode) {
		log.Printf("[Flipswitch] Failed to evaluate all flags: %d", resp.StatusCode)
		return results
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Flipswitch] Error reading response: %v", err)
		return results
	}

	var data map[string]interface{}
	if err := json.Unmarshal(respBody, &data); err != nil {
		log.Printf("[Flipswitch] Error parsing response: %v", err)
		return results
	}

	if flags, ok := data["flags"].([]interface{}); ok {
		for _, f := range flags {
			if flag, ok := f.(map[string]interface{}); ok {
				if key, ok := flag["key"].(string); ok {
					results = append(results, FlagEvaluation{
						Key:       key,
						Value:     flag["value"],
						ValueType: getFlagType(flag),
						Reason:    getString(flag, "reason", ""),
						Variant:   getString(flag, "variant", ""),
					})
				}
			}
		}
	}

	return results
}

// EvaluateFlag evaluates a single flag and returns its evaluation result.
// Returns nil if the flag doesn't exist.
//
// Note: This method makes direct HTTP calls for demo purposes.
// For standard flag evaluation, use the OpenFeature client methods.
func (p *FlipswitchProvider) EvaluateFlag(flagKey string, evalCtx openfeature.FlattenedContext) *FlagEvaluation {
	url := p.baseURL + "/ofrep/v1/evaluate/flags/" + flagKey

	body := map[string]interface{}{
		"context": transformContext(evalCtx),
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Printf("[Flipswitch] Error evaluating flag '%s': %v", flagKey, err)
		return nil
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", p.apiKey)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		log.Printf("[Flipswitch] Error evaluating flag '%s': %v", flagKey, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil
	}

	if !isSuccess(resp.StatusCode) {
		log.Printf("[Flipswitch] Failed to evaluate flag '%s': %d", flagKey, resp.StatusCode)
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[Flipswitch] Error reading response: %v", err)
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(respBody, &data); err != nil {
		log.Printf("[Flipswitch] Error parsing response: %v", err)
		return nil
	}

	return &FlagEvaluation{
		Key:       getString(data, "key", flagKey),
		Value:     data["value"],
		ValueType: getFlagType(data),
		Reason:    getString(data, "reason", ""),
		Variant:   getString(data, "variant", ""),
	}
}
