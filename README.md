# Flipswitch Go SDK

[![CI](https://github.com/flipswitch-io/go-sdk/actions/workflows/ci.yml/badge.svg)](https://github.com/flipswitch-io/go-sdk/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/flipswitch-io/go-sdk.svg)](https://pkg.go.dev/github.com/flipswitch-io/go-sdk)
[![codecov](https://codecov.io/gh/flipswitch-io/go-sdk/branch/main/graph/badge.svg)](https://codecov.io/gh/flipswitch-io/go-sdk)

Flipswitch SDK for Go with real-time SSE support for OpenFeature.

This SDK provides an OpenFeature-compatible provider that wraps OFREP flag evaluation with automatic cache invalidation via Server-Sent Events (SSE). When flags change in your Flipswitch dashboard, connected clients receive updates in real-time.

## Overview

- **OpenFeature Compatible**: Works with the OpenFeature standard for feature flags
- **Real-Time Updates**: SSE connection delivers instant flag changes
- **Polling Fallback**: Automatic fallback when SSE connection fails
- **Context-based Cancellation**: Proper Go idioms with context support

## Requirements

- Go 1.24+
- `github.com/open-feature/go-sdk`
- `github.com/open-feature/go-sdk-contrib/providers/ofrep`

## Installation

```bash
go get github.com/flipswitch-io/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/open-feature/go-sdk/openfeature"
    flipswitch "github.com/flipswitch-io/go-sdk"
)

func main() {
    // Create provider with API key
    provider, err := flipswitch.NewProvider("your-environment-api-key")
    if err != nil {
        log.Fatal(err)
    }
    defer provider.Shutdown()

    // Initialize the provider
    if err := provider.Init(openfeature.EvaluationContext{}); err != nil {
        log.Fatal(err)
    }

    // Register with OpenFeature
    openfeature.SetProvider(provider)
    client := openfeature.NewClient("my-app")

    // Evaluate flags
    ctx := context.Background()
    darkMode, _ := client.BooleanValue(ctx, "dark-mode", false, openfeature.EvaluationContext{})
    fmt.Printf("Dark mode: %v\n", darkMode)
}
```

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `apiKey` | `string` | *required* | Environment API key from dashboard |
| `WithBaseURL` | `string` | `https://api.flipswitch.io` | Your Flipswitch server URL |
| `WithRealtime` | `bool` | `true` | Enable SSE for real-time flag updates |
| `WithHTTPClient` | `*http.Client` | default | Custom HTTP client |
| `WithPollingFallback` | `bool` | `true` | Fall back to polling when SSE fails |
| `WithPollingInterval` | `time.Duration` | `30s` | Polling interval for fallback mode |
| `WithMaxSseRetries` | `int` | `5` | Max SSE retries before polling fallback |

```go
provider, err := flipswitch.NewProvider(
    "your-api-key",
    flipswitch.WithBaseURL("https://custom.server.com"),
    flipswitch.WithRealtime(true),
    flipswitch.WithPollingFallback(true),
    flipswitch.WithPollingInterval(30 * time.Second),
    flipswitch.WithMaxSseRetries(5),
)
```

## Usage Examples

### Basic Flag Evaluation

```go
ctx := context.Background()
client := openfeature.NewClient("my-app")

// Boolean flag
darkMode, _ := client.BooleanValue(ctx, "dark-mode", false, openfeature.EvaluationContext{})

// String flag
welcomeMsg, _ := client.StringValue(ctx, "welcome-message", "Hello!", openfeature.EvaluationContext{})

// Integer flag
maxItems, _ := client.IntValue(ctx, "max-items", 10, openfeature.EvaluationContext{})

// Float flag
discount, _ := client.FloatValue(ctx, "discount-rate", 0.0, openfeature.EvaluationContext{})

// Object flag
config, _ := client.ObjectValue(ctx, "feature-config", map[string]interface{}{}, openfeature.EvaluationContext{})
```

### Evaluation Context

Target specific users or segments:

```go
evalCtx := openfeature.NewEvaluationContext(
    "user-123",  // targeting key
    map[string]interface{}{
        "email":     "user@example.com",
        "plan":      "premium",
        "country":   "US",
        "beta_user": true,
    },
)

showFeature, _ := client.BooleanValue(ctx, "new-feature", false, evalCtx)
```

### Real-Time Updates (SSE)

Listen for flag changes:

```go
provider.AddFlagChangeListener(func(event flipswitch.FlagChangeEvent) {
    if event.FlagKey != "" {
        fmt.Printf("Flag changed: %s\n", event.FlagKey)
    } else {
        fmt.Println("All flags invalidated")
    }
})

// Check SSE status
status := provider.GetSseStatus()
// StatusConnecting, StatusConnected, StatusDisconnected, StatusError

// Force reconnect
provider.ReconnectSse()
```

### Bulk Flag Evaluation

Evaluate all flags at once:

```go
flags := provider.EvaluateAllFlags(openfeature.FlattenedContext{
    "targetingKey": "user-123",
    "email":        "user@example.com",
})

for _, flag := range flags {
    fmt.Printf("%s (%s): %s\n", flag.Key, flag.ValueType, flag.GetValueAsString())
    fmt.Printf("  Reason: %s, Variant: %s\n", flag.Reason, flag.Variant)
}

// Single flag with full details
flag := provider.EvaluateFlag("dark-mode", openfeature.FlattenedContext{"targetingKey": "user-123"})
if flag != nil {
    fmt.Printf("Value: %v\n", flag.Value)
    fmt.Printf("Reason: %s\n", flag.Reason)
    fmt.Printf("Variant: %s\n", flag.Variant)
}
```

## Advanced Features

### Polling Fallback

When SSE connection fails repeatedly, the SDK falls back to polling:

```go
provider, err := flipswitch.NewProvider(
    "your-api-key",
    flipswitch.WithPollingFallback(true),  // default: true
    flipswitch.WithPollingInterval(30 * time.Second),
    flipswitch.WithMaxSseRetries(5),
)

// Check if polling is active
if provider.IsPollingActive() {
    fmt.Println("Polling fallback is active")
}
```

### Custom HTTP Client

Provide a custom HTTP client:

```go
customClient := &http.Client{
    Timeout: 60 * time.Second,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        IdleConnTimeout:     90 * time.Second,
    },
}

provider, err := flipswitch.NewProvider(
    "your-api-key",
    flipswitch.WithHTTPClient(customClient),
)
```

### Context Cancellation

Use Go contexts for proper cancellation:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

value, err := client.BooleanValue(ctx, "my-flag", false, openfeature.EvaluationContext{})
if err != nil {
    // Handle timeout or cancellation
}
```

## Framework Integration

### HTTP Handler

```go
func myHandler(w http.ResponseWriter, r *http.Request) {
    client := openfeature.NewClient("web-app")

    userID := r.Header.Get("X-User-ID")
    evalCtx := openfeature.NewEvaluationContext(userID, nil)

    if darkMode, _ := client.BooleanValue(r.Context(), "dark-mode", false, evalCtx); darkMode {
        // Serve dark theme
    }
}
```

### Gin Middleware

```go
func FeatureFlagMiddleware() gin.HandlerFunc {
    client := openfeature.NewClient("gin-app")

    return func(c *gin.Context) {
        userID := c.GetHeader("X-User-ID")
        evalCtx := openfeature.NewEvaluationContext(userID, nil)

        maintenanceMode, _ := client.BooleanValue(c.Request.Context(), "maintenance-mode", false, evalCtx)
        if maintenanceMode {
            c.AbortWithStatus(http.StatusServiceUnavailable)
            return
        }

        c.Next()
    }
}
```

## Error Handling

The SDK handles errors gracefully:

```go
provider, err := flipswitch.NewProvider("your-api-key")
if err != nil {
    log.Fatalf("Failed to create provider: %v", err)
}

if err := provider.Init(openfeature.EvaluationContext{}); err != nil {
    log.Fatalf("Failed to initialize: %v", err)
}

// Flag evaluation returns default value on error
value, err := client.BooleanValue(ctx, "my-flag", false, evalCtx)
if err != nil {
    log.Printf("Evaluation error: %v", err)
    // value will be the default (false)
}
```

## Logging

The SDK uses Go's standard log package:

```go
// You'll see logs like:
// [Flipswitch] Provider initialized (realtime=true)
// [Flipswitch] SSE connection established
// [Flipswitch] SSE connection error, provider is stale
// [Flipswitch] Starting polling fallback
```

## Testing

Mock the provider in your tests:

```go
import "github.com/open-feature/go-sdk/openfeature/provider"

func TestWithMockFlags(t *testing.T) {
    // Use InMemoryProvider for testing
    flags := map[string]interface{}{
        "dark-mode": true,
        "max-items": 10,
    }
    openfeature.SetProvider(provider.NewInMemoryProvider(flags))

    client := openfeature.NewClient("test")
    value, _ := client.BooleanValue(context.Background(), "dark-mode", false, openfeature.EvaluationContext{})

    if value != true {
        t.Errorf("Expected true, got %v", value)
    }
}
```

## API Reference

### FlipswitchProvider

```go
type FlipswitchProvider struct {
    // ...
}

// Constructor
func NewProvider(apiKey string, opts ...Option) (*FlipswitchProvider, error)

// OpenFeature Provider interface
func (p *FlipswitchProvider) Metadata() openfeature.Metadata
func (p *FlipswitchProvider) Init(evaluationContext openfeature.EvaluationContext) error
func (p *FlipswitchProvider) Shutdown()
func (p *FlipswitchProvider) BooleanEvaluation(...) openfeature.BoolResolutionDetail
func (p *FlipswitchProvider) StringEvaluation(...) openfeature.StringResolutionDetail
func (p *FlipswitchProvider) FloatEvaluation(...) openfeature.FloatResolutionDetail
func (p *FlipswitchProvider) IntEvaluation(...) openfeature.IntResolutionDetail
func (p *FlipswitchProvider) ObjectEvaluation(...) openfeature.InterfaceResolutionDetail
func (p *FlipswitchProvider) Hooks() []openfeature.Hook

// Flipswitch-specific methods
func (p *FlipswitchProvider) GetSseStatus() ConnectionStatus
func (p *FlipswitchProvider) ReconnectSse()
func (p *FlipswitchProvider) IsPollingActive() bool
func (p *FlipswitchProvider) AddFlagChangeListener(handler FlagChangeHandler)
func (p *FlipswitchProvider) RemoveFlagChangeListener(handler FlagChangeHandler)
func (p *FlipswitchProvider) EvaluateAllFlags(evalCtx openfeature.FlattenedContext) []FlagEvaluation
func (p *FlipswitchProvider) EvaluateFlag(flagKey string, evalCtx openfeature.FlattenedContext) *FlagEvaluation
```

### Types

```go
type ConnectionStatus string

const (
    StatusConnecting   ConnectionStatus = "connecting"
    StatusConnected    ConnectionStatus = "connected"
    StatusDisconnected ConnectionStatus = "disconnected"
    StatusError        ConnectionStatus = "error"
)

type FlagChangeEvent struct {
    FlagKey   string // empty for bulk invalidation
    Timestamp string
}

type FlagChangeHandler func(event FlagChangeEvent)

type FlagEvaluation struct {
    Key       string
    Value     interface{}
    ValueType string
    Reason    string
    Variant   string
}
```

## Troubleshooting

### SSE Connection Fails

- Check that your API key is valid
- Verify your server URL is correct
- Check for network/firewall issues blocking SSE
- The SDK will automatically fall back to polling

### Flags Not Updating in Real-Time

- Ensure `WithRealtime(true)` is set (default)
- Check SSE status with `provider.GetSseStatus()`
- Check logs for error messages

### Provider Initialization Fails

- Verify your API key is correct
- Check network connectivity to the Flipswitch server
- Review logs for detailed error messages

## Demo

Run the included demo:

```bash
cd examples/demo
go run main.go <your-api-key>
```

The demo will connect, display all flags, and listen for real-time updates.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

MIT - see [LICENSE](LICENSE) for details.
