# Flipswitch Go SDK

Flipswitch SDK for Go with real-time SSE support for OpenFeature.

## Installation

```bash
go get github.com/flipswitch-dev/flipswitch/sdks/go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "github.com/open-feature/go-sdk/openfeature"
    flipswitch "github.com/flipswitch-dev/flipswitch/sdks/go"
)

func main() {
    // Only API key is required
    provider, err := flipswitch.NewProvider("YOUR_API_KEY")
    if err != nil {
        panic(err)
    }
    defer provider.Shutdown()

    // Initialize the provider
    if err := provider.Init(openfeature.EvaluationContext{}); err != nil {
        panic(err)
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
| `WithBaseURL` | `string` | `https://api.flipswitch.dev` | Your Flipswitch server URL |
| `WithRealtime` | `bool` | `true` | Enable SSE for real-time flag updates |

```go
provider, err := flipswitch.NewProvider(
    "YOUR_API_KEY",
    flipswitch.WithBaseURL("https://api.flipswitch.dev"),
    flipswitch.WithRealtime(true),
)
```

## Evaluation Context

Pass user attributes for targeting:

```go
evalCtx := openfeature.NewEvaluationContext(
    "user-123",  // targeting key
    map[string]interface{}{
        "email":   "user@example.com",
        "plan":    "premium",
        "country": "SE",
    },
)

showFeature, _ := client.BooleanValue(ctx, "new-feature", false, evalCtx)
```

## Real-Time Updates

When `WithRealtime(true)` (default), the SDK maintains an SSE connection to receive instant flag changes:

### Event Listeners

```go
provider.AddFlagChangeListener(func(event flipswitch.FlagChangeEvent) {
    fmt.Printf("Flag changed: %s\n", event.FlagKey)
})
```

### Connection Status

```go
// Check current SSE status
status := provider.GetSseStatus()
// StatusConnecting, StatusConnected, StatusDisconnected, StatusError

// Force reconnect
provider.ReconnectSse()
```

## Bulk Flag Evaluation

Evaluate all flags at once:

```go
// Evaluate all flags
flags := provider.EvaluateAllFlags(openfeature.FlattenedContext{
    "targetingKey": "user-123",
})
for _, flag := range flags {
    fmt.Printf("%s (%s): %s\n", flag.Key, flag.ValueType, flag.GetValueAsString())
}

// Evaluate a single flag with full details
flag := provider.EvaluateFlag("dark-mode", context)
if flag != nil {
    fmt.Printf("Value: %v, Reason: %s, Variant: %s\n", flag.Value, flag.Reason, flag.Variant)
}
```

## Shutdown

Always shutdown the provider when done:

```go
provider.Shutdown()
```

## Development

```bash
# Download dependencies
go mod tidy

# Build
go build ./...

# Run tests
go test ./...

# Run demo
cd examples/demo
go run main.go <api-key>
```

## License

MIT
