package types

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewOutput(t *testing.T) {
	o := NewOutput()
	if o.FetchedAt == "" {
		t.Fatal("expected non-empty FetchedAt")
	}
	if o.Error != "" {
		t.Fatalf("expected empty error, got %q", o.Error)
	}
	// Verify it's a valid RFC3339 timestamp.
	_, err := time.Parse(time.RFC3339, o.FetchedAt)
	if err != nil {
		t.Fatalf("invalid RFC3339 timestamp: %v", err)
	}
}

func TestErrorOutput(t *testing.T) {
	msg := "something went wrong"
	o := ErrorOutput(msg)
	if o.Error != msg {
		t.Fatalf("expected error %q, got %q", msg, o.Error)
	}
	if o.FetchedAt == "" {
		t.Fatal("expected non-empty FetchedAt")
	}
	if o.Rolling != nil || o.Weekly != nil || o.Monthly != nil {
		t.Fatal("expected nil data fields on error output")
	}
}

func TestOutputJSON_Success(t *testing.T) {
	pct := 42
	o := Output{
		Rolling:           &UsageData{Status: "ok", ResetInSec: 1000, UsagePercent: pct},
		Weekly:            &UsageData{Status: "ok", ResetInSec: 2000, UsagePercent: 10},
		Monthly:           &UsageData{Status: "ok", ResetInSec: 3000, UsagePercent: 5},
		BalanceMicroCents: int64Ptr(1000000000),
		FetchedAt:         "2026-07-11T21:00:00Z",
	}

	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify fields present.
	out := string(b)
	for _, want := range []string{
		`"rolling"`, `"weekly"`, `"monthly"`,
		`"status": "ok"`, `"usage_percent": 42`,
		`"balance_microcents": 1000000000`,
		`"fetched_at"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected JSON to contain %q", want)
		}
	}

	// Verify error field is absent on success.
	if strings.Contains(out, `"error"`) {
		t.Error("expected no error field in success output")
	}
}

func TestOutputJSON_Error(t *testing.T) {
	o := ErrorOutput("test error")

	b, err := json.MarshalIndent(o, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	out := string(b)
	if !strings.Contains(out, `"error": "test error"`) {
		t.Errorf("expected error field, got: %s", out)
	}
	if strings.Contains(out, `"rolling"`) {
		t.Error("expected no rolling field on error output")
	}
}

func TestOutputJSON_UsageDataOmitEmpty(t *testing.T) {
	o := NewOutput()
	b, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := string(b)
	// Usage fields should be omitted when nil.
	if strings.Contains(out, `"rolling"`) {
		t.Error("expected rolling to be omitted when nil")
	}
	if strings.Contains(out, `"balance_microcents"`) {
		t.Error("expected balance_microcents to be omitted when nil")
	}
}

func int64Ptr(v int64) *int64 { return &v }
