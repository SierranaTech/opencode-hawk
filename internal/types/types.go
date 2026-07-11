// Package types defines the JSON contract between hawk and its consumers.
package types

import "time"

// UsageData represents one usage window scraped from the dashboard.
type UsageData struct {
	Status       string `json:"status"`
	ResetInSec   int64  `json:"reset_in_sec"`
	UsagePercent int    `json:"usage_percent"`
}

// Output is the JSON structure written to stdout on every invocation.
// On success, Error is empty. On failure, the data fields may be nil
// and Error describes the problem.
type Output struct {
	Rolling           *UsageData `json:"rolling,omitempty"`
	Weekly            *UsageData `json:"weekly,omitempty"`
	Monthly           *UsageData `json:"monthly,omitempty"`
	BalanceMicroCents *int64     `json:"balance_microcents,omitempty"`
	FetchedAt         string     `json:"fetched_at"`
	Error             string     `json:"error,omitempty"`
}

func NewOutput() Output {
	return Output{FetchedAt: time.Now().UTC().Format(time.RFC3339)}
}

func ErrorOutput(msg string) Output {
	o := NewOutput()
	o.Error = msg
	return o
}
