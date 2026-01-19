package flipswitch

import "time"

// FlipswitchOptions contains configuration options for the Flipswitch provider.
type FlipswitchOptions struct {
	// APIKey is the environment API key (required).
	APIKey string

	// BaseURL is the Flipswitch server URL.
	// Default: "https://api.flipswitch.dev"
	BaseURL string

	// EnableRealtime enables SSE for real-time flag updates.
	// Default: true
	EnableRealtime bool
}

// ConnectionStatus represents the SSE connection status.
type ConnectionStatus string

const (
	// StatusConnecting indicates the client is connecting.
	StatusConnecting ConnectionStatus = "connecting"
	// StatusConnected indicates the client is connected.
	StatusConnected ConnectionStatus = "connected"
	// StatusDisconnected indicates the client is disconnected.
	StatusDisconnected ConnectionStatus = "disconnected"
	// StatusError indicates there was a connection error.
	StatusError ConnectionStatus = "error"
)

// FlagChangeEvent represents a flag change event received via SSE.
type FlagChangeEvent struct {
	// FlagKey is the key of the flag that changed, or empty for bulk invalidation.
	FlagKey string `json:"flagKey,omitempty"`

	// Timestamp is the ISO timestamp of when the change occurred.
	Timestamp string `json:"timestamp"`
}

// GetTimestampAsTime returns the timestamp as a time.Time object.
func (e *FlagChangeEvent) GetTimestampAsTime() (time.Time, error) {
	if e.Timestamp == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, e.Timestamp)
}

// FlagEvaluation represents the result of evaluating a single flag.
type FlagEvaluation struct {
	// Key is the flag key.
	Key string

	// Value is the evaluated value.
	Value interface{}

	// ValueType is the type of the value (boolean, string, number, etc.).
	ValueType string

	// Reason is the reason for this evaluation result.
	Reason string

	// Variant is the variant that matched, if applicable.
	Variant string
}

// AsBoolean returns the value as a boolean.
func (e *FlagEvaluation) AsBoolean() bool {
	if b, ok := e.Value.(bool); ok {
		return b
	}
	return false
}

// AsInt returns the value as an integer.
func (e *FlagEvaluation) AsInt() int {
	switch v := e.Value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// AsFloat returns the value as a float64.
func (e *FlagEvaluation) AsFloat() float64 {
	switch v := e.Value.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0.0
}

// AsString returns the value as a string.
func (e *FlagEvaluation) AsString() string {
	if s, ok := e.Value.(string); ok {
		return s
	}
	return ""
}

// GetValueAsString returns the value formatted for display.
func (e *FlagEvaluation) GetValueAsString() string {
	if e.Value == nil {
		return "null"
	}
	switch v := e.Value.(type) {
	case string:
		return "\"" + v + "\""
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return stringValue(v)
	}
}

func stringValue(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return intToString(val)
	case int64:
		return int64ToString(val)
	case float64:
		return floatToString(val)
	default:
		return ""
	}
}

func intToString(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

func int64ToString(i int64) string {
	return intToString(int(i))
}

func floatToString(f float64) string {
	// Simple float to string conversion
	if f == float64(int(f)) {
		return intToString(int(f))
	}
	// For non-integer floats, use a simple approach
	intPart := int(f)
	fracPart := f - float64(intPart)
	if fracPart < 0 {
		fracPart = -fracPart
	}
	// Convert to 2 decimal places
	fracInt := int(fracPart * 100)
	if fracInt == 0 {
		return intToString(intPart)
	}
	return intToString(intPart) + "." + intToString(fracInt)
}

// FlagChangeHandler is called when a flag changes.
type FlagChangeHandler func(event FlagChangeEvent)

// ConnectionStatusHandler is called when the SSE connection status changes.
type ConnectionStatusHandler func(status ConnectionStatus)
