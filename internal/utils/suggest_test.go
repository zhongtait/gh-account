package utils

import "testing"

func TestSuggestCandidate(t *testing.T) {
	candidates := []string{"personal", "work-dev", "work-prod", "staging"}

	// 1. One letter off
	if got := SuggestCandidate("personl", candidates); got != "personal" {
		t.Fatalf("expected personal, got: %s", got)
	}

	// 2. Transposed or off by 1
	if got := SuggestCandidate("work-de", candidates); got != "work-dev" {
		t.Fatalf("expected work-dev, got: %s", got)
	}

	// 3. Exact match returns empty (no need to suggest something that is already identical)
	if got := SuggestCandidate("personal", candidates); got != "" {
		t.Fatalf("expected empty for exact match, got: %s", got)
	}

	// 4. Completely unrelated
	if got := SuggestCandidate("completely-different-long-word", candidates); got != "" {
		t.Fatalf("expected empty for distant word, got: %s", got)
	}

	// 5. Empty inputs
	if got := SuggestCandidate("", candidates); got != "" {
		t.Fatalf("expected empty for empty input, got: %s", got)
	}
	if got := SuggestCandidate("test", nil); got != "" {
		t.Fatalf("expected empty for nil candidates, got: %s", got)
	}
}
