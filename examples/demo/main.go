// Sample application demonstrating Flipswitch integration with real-time SSE support.
//
// Run this demo with:
//
//	go run main.go <api-key> [base-url]
//
// Or set the FLIPSWITCH_BASE_URL environment variable:
//
//	FLIPSWITCH_BASE_URL=http://localhost:8080 go run main.go <api-key>
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/open-feature/go-sdk/openfeature"

	flipswitch "github.com/flipswitch-io/go-sdk"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: go run main.go <api-key> [base-url]")
		fmt.Fprintln(os.Stderr, "       Or set FLIPSWITCH_BASE_URL environment variable")
		os.Exit(1)
	}

	apiKey := os.Args[1]

	// Get base URL from command line or environment variable
	baseURL := ""
	if len(os.Args) >= 3 {
		baseURL = os.Args[2]
	} else if envURL := os.Getenv("FLIPSWITCH_BASE_URL"); envURL != "" {
		baseURL = envURL
	}

	fmt.Println("Flipswitch Go SDK Demo")
	fmt.Println("======================")
	fmt.Println()

	// Build provider options
	var opts []flipswitch.Option
	if baseURL != "" {
		fmt.Printf("Using base URL: %s\n", baseURL)
		opts = append(opts, flipswitch.WithBaseURL(baseURL))
	}

	// API key is required, all other options have sensible defaults
	provider, err := flipswitch.NewProvider(apiKey, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create provider: %v\n", err)
		os.Exit(1)
	}
	defer provider.Shutdown()

	// Initialize the provider
	err = provider.Init(openfeature.EvaluationContext{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Flipswitch: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Connected! SSE Status: %s\n", provider.GetSseStatus())

	// Register the provider with OpenFeature
	openfeature.SetProvider(provider)

	// Create evaluation context with user information
	context := openfeature.FlattenedContext{
		"targetingKey": "user-123",
		"email":        "user@example.com",
		"plan":         "premium",
		"country":      "SE",
	}

	// Add a listener for flag changes - re-evaluate and show new value
	provider.AddFlagChangeListener(func(event flipswitch.FlagChangeEvent) {
		flagKey := event.FlagKey
		if flagKey == "" {
			flagKey = "all flags"
		}
		fmt.Printf("\n*** Flag changed: %s ***\n", flagKey)

		if event.FlagKey != "" {
			// Re-evaluate the specific flag that changed
			eval := provider.EvaluateFlag(event.FlagKey, context)
			if eval != nil {
				printFlag(*eval)
			}
		} else {
			// Bulk invalidation - re-evaluate all flags
			printAllFlags(provider, context)
		}
		fmt.Println()
	})

	fmt.Println("\nEvaluating flags for user: user-123")
	fmt.Println("Context: email=user@example.com, plan=premium, country=SE")
	fmt.Println()

	printAllFlags(provider, context)

	// Keep the application running to demonstrate real-time updates
	fmt.Println("\n--- Listening for real-time flag updates (Ctrl+C to exit) ---")
	fmt.Println("Change a flag in the Flipswitch dashboard to see it here!")
	fmt.Println()

	// Keep running for 5 minutes to demonstrate real-time updates
	time.Sleep(5 * time.Minute)

	fmt.Println("\nDemo complete!")
}

func printAllFlags(provider *flipswitch.FlipswitchProvider, context openfeature.FlattenedContext) {
	flags := provider.EvaluateAllFlags(context)

	if len(flags) == 0 {
		fmt.Println("No flags found.")
		return
	}

	fmt.Printf("Flags (%d):\n", len(flags))
	fmt.Println(strings.Repeat("-", 60))

	for _, flag := range flags {
		printFlag(flag)
	}
}

func printFlag(flag flipswitch.FlagEvaluation) {
	variantStr := ""
	if flag.Variant != "" {
		variantStr = ", variant=" + flag.Variant
	}
	fmt.Printf("  %-30s (%s) = %s\n", flag.Key, flag.ValueType, flag.GetValueAsString())
	fmt.Printf("    └─ reason=%s%s\n", flag.Reason, variantStr)
}
