package skills

import (
	"strings"
	"testing"
)

func TestH3BundleIsComplete(t *testing.T) {
	for _, file := range []string{"SKILL.md", "references/base-en.txt", "references/ref-en.txt"} {
		body, err := Read("h3-prompt-writing", file)
		if err != nil || len(body) < 100 {
			t.Fatalf("read bundled %s: %v", file, err)
		}
	}
	ref, _ := Read("h3-prompt-writing", "references/ref-en.txt")
	if !strings.Contains(ref, "subject_definitions") || !strings.Contains(ref, "retention_analysis") {
		t.Fatal("reference mode guide lost its required sections")
	}
	if _, err := Read("h3-prompt-writing", "../secret"); err == nil {
		t.Fatal("parent traversal must be rejected")
	}
}
