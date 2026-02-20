package task

import (
	"strings"
	"testing"
)

func TestGenerateTaskID(t *testing.T) {
	toolName := "upgrade"
	id := GenerateTaskID(toolName)

	if !strings.HasPrefix(id, "task-upgrade-") {
		t.Errorf("expected prefix task-upgrade-, got %s", id)
	}

	parts := strings.Split(id, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 parts in ID, got %d: %v", len(parts), parts)
	}

	// Verify Title Case for adjectives and nouns
	for i := 2; i < 5; i++ {
		word := parts[i]
		if len(word) == 0 {
			t.Errorf("part %d is empty", i)
			continue
		}
		if word[0] < 'A' || word[0] > 'Z' {
			t.Errorf("part %d (%s) does not start with a capital letter", i, word)
		}
		if len(word) > 1 {
			for _, r := range word[1:] {
				if r >= 'A' && r <= 'Z' {
					t.Errorf("part %d (%s) has capital letter after the first", i, word)
				}
			}
		}
	}
}

func TestWordListsUniqueness(t *testing.T) {
	checkUniqueness := func(name string, list []string) {
		seen := make(map[string]bool)
		for _, w := range list {
			lower := strings.ToLower(w)
			if seen[lower] {
				t.Errorf("duplicate word in %s: %s", name, w)
			}
			seen[lower] = true
		}
	}

	checkUniqueness("adjectives", adjectives)
	checkUniqueness("nouns", nouns)
}

func TestEntropy(t *testing.T) {
	combinations := float64(len(adjectives)) * float64(len(adjectives)) * float64(len(nouns))
	t.Logf("Total combinations: %f", combinations)
	if combinations < 1000000 {
		t.Errorf("too few combinations: %f", combinations)
	}
}
