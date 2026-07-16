//go:build integration

// Package integration contains tests that span multiple internal packages,
// verifying that config loading, validation, and generation work together
// correctly. These tests require no external binaries — they exercise
// the Go code directly.
//
// Run with: go test -tags=integration ./internal/e2e/...
package integration
