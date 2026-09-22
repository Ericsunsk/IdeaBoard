package domain

import (
	"errors"
	"time"
)

// Provider is a web image search supplier. Vision search never enters this
// vocabulary: it rides the central AI gateway without per-provider keys.
type Provider string

const (
	ProviderUnsplash Provider = "unsplash"
	ProviderPexels   Provider = "pexels"
)

// Providers lists every configurable web search supplier, in display order.
var Providers = []Provider{ProviderUnsplash, ProviderPexels}

var ErrProviderUnavailable = errors.New("search provider is not configured or enabled")
var ErrProviderTestRequired = errors.New("search provider must pass a connection test before enabling")

// IsProvider reports whether the value names a supported web search supplier.

func IsProvider(provider Provider) bool {
	return provider == ProviderUnsplash || provider == ProviderPexels
}

// TestState is a provider connection-test lifecycle state.
type TestState string

const (
	TestStateNever  TestState = "never"
	TestStatePassed TestState = "passed"
	TestStateFailed TestState = "failed"
)

// FailureCategory classifies provider test failures for admin display.
type FailureCategory string

const (
	FailureCredentials FailureCategory = "credentials"
	FailureQuota       FailureCategory = "quota_or_permission"
	FailureNetwork     FailureCategory = "network_timeout"
	FailureResponse    FailureCategory = "response_format"
	FailureUnknown     FailureCategory = "unknown"
)

// ProviderConfig is the admin-visible web search provider configuration.
// API keys never appear here, only whether one is configured.
type ProviderConfig struct {
	Provider         Provider        `json:"provider"`
	APIKeyConfigured bool            `json:"api_key_configured"`
	EndpointURL      string          `json:"endpoint_url,omitempty"`
	Model            string          `json:"model,omitempty"`
	Enabled          bool            `json:"enabled"`
	TestState        TestState       `json:"test_state"`
	FailureCategory  FailureCategory `json:"failure_category,omitempty"`
	StatusMessage    string          `json:"status_message,omitempty"`
	TestedAt         *time.Time      `json:"tested_at,omitempty"`
}
