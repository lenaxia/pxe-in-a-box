//go:build e2e

// Package e2e contains end-to-end tests that start a real matchbox server
// with generated configurations and verify HTTP responses over the network.
//
// These tests require the matchbox binary to be available on PATH.
// In CI, matchbox is downloaded automatically. Locally, install it or run:
//
//	make test-e2e
//
// Run with: go test -tags=e2e ./test/e2e/...
package e2e
