package projection

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateSummaryKeepsUTF8Valid(t *testing.T) {
	t.Parallel()

	input := "I’m adding the stdio MCP binary and a small in-memory transport test suite now. That should give us the same confidence your other repos rely on."
	got := truncateSummary(input, 80)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateSummary() produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("truncateSummary() = %q, want ellipsis suffix", got)
	}
}
