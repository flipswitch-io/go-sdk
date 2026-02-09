package flipswitch

import (
	"testing"
	"time"
)

// ========================================
// GetTimestampAsTime Tests
// ========================================

func TestGetTimestampAsTime_Valid(t *testing.T) {
	e := &FlagChangeEvent{Timestamp: "2024-01-15T10:30:00Z"}
	got, err := e.GetTimestampAsTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}

func TestGetTimestampAsTime_Empty(t *testing.T) {
	e := &FlagChangeEvent{Timestamp: ""}
	got, err := e.GetTimestampAsTime()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}
}

func TestGetTimestampAsTime_Invalid(t *testing.T) {
	e := &FlagChangeEvent{Timestamp: "not-a-date"}
	_, err := e.GetTimestampAsTime()
	if err == nil {
		t.Fatal("expected error for invalid timestamp")
	}
}

// ========================================
// AsBoolean Tests
// ========================================

func TestAsBoolean_True(t *testing.T) {
	e := &FlagEvaluation{Value: true}
	if !e.AsBoolean() {
		t.Error("expected true")
	}
}

func TestAsBoolean_False(t *testing.T) {
	e := &FlagEvaluation{Value: false}
	if e.AsBoolean() {
		t.Error("expected false")
	}
}

func TestAsBoolean_NonBool(t *testing.T) {
	e := &FlagEvaluation{Value: "hello"}
	if e.AsBoolean() {
		t.Error("expected false for non-bool value")
	}
}

// ========================================
// AsInt Tests
// ========================================

func TestAsInt_Int(t *testing.T) {
	e := &FlagEvaluation{Value: 42}
	if got := e.AsInt(); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

func TestAsInt_Int64(t *testing.T) {
	e := &FlagEvaluation{Value: int64(99)}
	if got := e.AsInt(); got != 99 {
		t.Errorf("expected 99, got %d", got)
	}
}

func TestAsInt_Float64(t *testing.T) {
	e := &FlagEvaluation{Value: float64(3.7)}
	if got := e.AsInt(); got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}

func TestAsInt_NonNumber(t *testing.T) {
	e := &FlagEvaluation{Value: "abc"}
	if got := e.AsInt(); got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// ========================================
// AsFloat Tests
// ========================================

func TestAsFloat_Float64(t *testing.T) {
	e := &FlagEvaluation{Value: float64(3.14)}
	if got := e.AsFloat(); got != 3.14 {
		t.Errorf("expected 3.14, got %f", got)
	}
}

func TestAsFloat_Int(t *testing.T) {
	e := &FlagEvaluation{Value: 42}
	if got := e.AsFloat(); got != 42.0 {
		t.Errorf("expected 42.0, got %f", got)
	}
}

func TestAsFloat_Int64(t *testing.T) {
	e := &FlagEvaluation{Value: int64(99)}
	if got := e.AsFloat(); got != 99.0 {
		t.Errorf("expected 99.0, got %f", got)
	}
}

func TestAsFloat_NonNumber(t *testing.T) {
	e := &FlagEvaluation{Value: "abc"}
	if got := e.AsFloat(); got != 0.0 {
		t.Errorf("expected 0.0, got %f", got)
	}
}

// ========================================
// AsString Tests
// ========================================

func TestAsString_String(t *testing.T) {
	e := &FlagEvaluation{Value: "hello"}
	if got := e.AsString(); got != "hello" {
		t.Errorf("expected 'hello', got '%s'", got)
	}
}

func TestAsString_NonString(t *testing.T) {
	e := &FlagEvaluation{Value: 42}
	if got := e.AsString(); got != "" {
		t.Errorf("expected empty string, got '%s'", got)
	}
}

// ========================================
// GetValueAsString Tests
// ========================================

func TestGetValueAsString_Null(t *testing.T) {
	e := &FlagEvaluation{Value: nil}
	if got := e.GetValueAsString(); got != "null" {
		t.Errorf("expected 'null', got '%s'", got)
	}
}

func TestGetValueAsString_String(t *testing.T) {
	e := &FlagEvaluation{Value: "hi"}
	if got := e.GetValueAsString(); got != "\"hi\"" {
		t.Errorf("expected '\"hi\"', got '%s'", got)
	}
}

func TestGetValueAsString_BoolTrue(t *testing.T) {
	e := &FlagEvaluation{Value: true}
	if got := e.GetValueAsString(); got != "true" {
		t.Errorf("expected 'true', got '%s'", got)
	}
}

func TestGetValueAsString_BoolFalse(t *testing.T) {
	e := &FlagEvaluation{Value: false}
	if got := e.GetValueAsString(); got != "false" {
		t.Errorf("expected 'false', got '%s'", got)
	}
}

func TestGetValueAsString_Int(t *testing.T) {
	e := &FlagEvaluation{Value: 42}
	if got := e.GetValueAsString(); got != "42" {
		t.Errorf("expected '42', got '%s'", got)
	}
}

func TestGetValueAsString_NegativeInt(t *testing.T) {
	e := &FlagEvaluation{Value: -7}
	if got := e.GetValueAsString(); got != "-7" {
		t.Errorf("expected '-7', got '%s'", got)
	}
}

func TestGetValueAsString_Int64(t *testing.T) {
	e := &FlagEvaluation{Value: int64(100)}
	if got := e.GetValueAsString(); got != "100" {
		t.Errorf("expected '100', got '%s'", got)
	}
}

func TestGetValueAsString_Float(t *testing.T) {
	e := &FlagEvaluation{Value: float64(3.14)}
	if got := e.GetValueAsString(); got != "3.14" {
		t.Errorf("expected '3.14', got '%s'", got)
	}
}

func TestGetValueAsString_FloatInteger(t *testing.T) {
	e := &FlagEvaluation{Value: float64(5.0)}
	if got := e.GetValueAsString(); got != "5" {
		t.Errorf("expected '5', got '%s'", got)
	}
}

func TestGetValueAsString_Unknown(t *testing.T) {
	e := &FlagEvaluation{Value: struct{}{}}
	if got := e.GetValueAsString(); got != "" {
		t.Errorf("expected empty string for unknown type, got '%s'", got)
	}
}

// ========================================
// stringValue Tests (through GetValueAsString default branch)
// ========================================

func TestStringValue_String(t *testing.T) {
	// stringValue with a string input returns the raw string (no quotes)
	got := stringValue("hello")
	if got != "hello" {
		t.Errorf("expected 'hello', got '%s'", got)
	}
}

func TestStringValue_BoolTrue(t *testing.T) {
	got := stringValue(true)
	if got != "true" {
		t.Errorf("expected 'true', got '%s'", got)
	}
}

func TestStringValue_BoolFalse(t *testing.T) {
	got := stringValue(false)
	if got != "false" {
		t.Errorf("expected 'false', got '%s'", got)
	}
}

// ========================================
// intToString Tests
// ========================================

func TestIntToString_Zero(t *testing.T) {
	if got := intToString(0); got != "0" {
		t.Errorf("expected '0', got '%s'", got)
	}
}

func TestIntToString_Positive(t *testing.T) {
	if got := intToString(123); got != "123" {
		t.Errorf("expected '123', got '%s'", got)
	}
}

func TestIntToString_Negative(t *testing.T) {
	if got := intToString(-42); got != "-42" {
		t.Errorf("expected '-42', got '%s'", got)
	}
}

// ========================================
// int64ToString Tests
// ========================================

func TestInt64ToString(t *testing.T) {
	if got := int64ToString(999); got != "999" {
		t.Errorf("expected '999', got '%s'", got)
	}
}

func TestInt64ToString_Negative(t *testing.T) {
	if got := int64ToString(-50); got != "-50" {
		t.Errorf("expected '-50', got '%s'", got)
	}
}

// ========================================
// floatToString Tests
// ========================================

func TestFloatToString_Integer(t *testing.T) {
	if got := floatToString(5.0); got != "5" {
		t.Errorf("expected '5', got '%s'", got)
	}
}

func TestFloatToString_WithDecimals(t *testing.T) {
	if got := floatToString(3.14); got != "3.14" {
		t.Errorf("expected '3.14', got '%s'", got)
	}
}

func TestFloatToString_Zero(t *testing.T) {
	if got := floatToString(0.0); got != "0" {
		t.Errorf("expected '0', got '%s'", got)
	}
}

func TestFloatToString_NegativeWithDecimals(t *testing.T) {
	got := floatToString(-2.50)
	if got != "-2.50" && got != "-2.5" {
		// The implementation converts -2.50: intPart=-2, fracPart=0.50, fracInt=50
		// So result is intToString(-2) + "." + intToString(50) = "-2.50"
		t.Errorf("expected '-2.50' or '-2.5', got '%s'", got)
	}
}
