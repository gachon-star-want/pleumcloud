package placement

import "testing"

type acct struct {
	id   string
	free int64
}

func TestDefaultPolicyPicksMostFreeSpace(t *testing.T) {
	accts := []Account{{ID: "g", Free: 10 << 30}, {ID: "m", Free: 25 << 30}}
	got := Decide(FileInfo{Name: "movie.mp4", Size: 1 << 30, MIME: "video/mp4"}, accts, nil)
	if got != "m" {
		t.Fatalf("got %q, want most-free account", got)
	}
}

func TestSizeMustFit(t *testing.T) {
	accts := []Account{{ID: "small", Free: 100}, {ID: "big", Free: 10 << 30}}
	got := Decide(FileInfo{Name: "huge.bin", Size: 1 << 30}, accts, nil)
	if got != "big" {
		t.Fatalf("got %q — oversized target must be skipped", got)
	}
}

func TestRuleOverridesDefault(t *testing.T) {
	accts := []Account{{ID: "g", Free: 10 << 30}, {ID: "m", Free: 25 << 30}}
	rules := []Rule{
		{Priority: 1, Field: "mime", Op: "is", Value: "video/", Target: "g"},
	}
	got := Decide(FileInfo{Name: "movie.mp4", Size: 1, MIME: "video/mp4"}, accts, rules)
	if got != "g" {
		t.Fatalf("got %q, want rule target", got)
	}
}

func TestRulesRunInPriorityOrder(t *testing.T) {
	accts := []Account{{ID: "a", Free: 1}, {ID: "b", Free: 1}}
	rules := []Rule{
		{Priority: 2, Field: "name", Op: "contains", Value: "photo", Target: "b"},
		{Priority: 1, Field: "name", Op: "startswith", Value: "2026", Target: "a"},
	}
	// name starts with 2026 AND contains photo → priority 1 wins.
	got := Decide(FileInfo{Name: "2026-photos.zip"}, accts, rules)
	if got != "a" {
		t.Fatalf("got %q, want priority-1 target", got)
	}
}

func TestSizeRules(t *testing.T) {
	accts := []Account{{ID: "big", Free: 1 << 40}, {ID: "fast", Free: 1 << 30}}
	rules := []Rule{{Priority: 1, Field: "size", Op: "gt", Value: "1073741824", Target: "big"}}
	if got := Decide(FileInfo{Name: "x.iso", Size: 5 << 30}, accts, rules); got != "big" {
		t.Fatalf("gt rule: got %q", got)
	}
	// Below the threshold: rule doesn't apply → most free (big) anyway; make
	// the alternative win to prove the rule stopped applying.
	accts[0].Free = 1 << 20
	if got := Decide(FileInfo{Name: "x.txt", Size: 10}, accts, rules); got != "fast" {
		t.Fatalf("below threshold: got %q", got)
	}
}

func TestRuleTargetMustExistAndFit(t *testing.T) {
	accts := []Account{{ID: "a", Free: 50}}
	rules := []Rule{{Priority: 1, Field: "name", Op: "contains", Value: "x", Target: "ghost"}}
	if got := Decide(FileInfo{Name: "x.bin", Size: 10}, accts, rules); got != "a" {
		t.Fatalf("ghost target should fall back, got %q", got)
	}
	rules[0].Target = "a"
	if got := Decide(FileInfo{Name: "x.bin", Size: 100}, accts, rules); got != "" {
		t.Fatalf("target too small should fail, got %q", got)
	}
}

func TestNoAccountsNoAnswer(t *testing.T) {
	if got := Decide(FileInfo{Name: "x"}, nil, nil); got != "" {
		t.Fatalf("got %q", got)
	}
}
