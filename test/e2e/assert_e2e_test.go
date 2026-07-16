//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		snippet := s
		if len(snippet) > 300 {
			snippet = s[:300] + "..."
		}
		t.Errorf("expected response to contain %q\nresponse:\n%s", substr, snippet)
	}
}
