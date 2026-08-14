// Package placement decides where new files go. The default policy picks
// the account with the most free space that can hold the file; user rules
// (evaluated in priority order) override it.
package placement

import (
	"sort"
	"strconv"
	"strings"
)

// Account is a placement candidate.
type Account struct {
	ID   string
	Free int64
}

// FileInfo describes the incoming file.
type FileInfo struct {
	Name string
	Size int64
	MIME string
}

// Rule is a user-configured placement rule. Fields: mime|name|size;
// Ops: is|startswith|contains|gt|lt; Target: account ID.
type Rule struct {
	Priority int
	Field    string
	Op       string
	Value    string
	Target   string
	Enabled  bool
}

// Decide returns the account ID that should receive the file ("" when no
// account fits).
func Decide(f FileInfo, accts []Account, rules []Rule) string {
	byID := map[string]Account{}
	for _, a := range accts {
		byID[a.ID] = a
	}

	ordered := make([]Rule, len(rules))
	copy(ordered, rules)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Priority < ordered[j].Priority })
	for _, r := range ordered {
		if !match(f, r) {
			continue
		}
		if tgt, ok := byID[r.Target]; ok && tgt.Free >= f.Size {
			return r.Target
		}
	}

	var best Account
	var bestID string
	for _, a := range accts {
		if a.Free < f.Size {
			continue
		}
		if a.Free > best.Free {
			best, bestID = a, a.ID
		}
	}
	return bestID
}

func match(f FileInfo, r Rule) bool {
	switch r.Field {
	case "mime":
		switch r.Op {
		case "is":
			return strings.HasPrefix(f.MIME, r.Value)
		case "contains":
			return strings.Contains(f.MIME, r.Value)
		}
	case "name":
		lower := strings.ToLower(f.Name)
		v := strings.ToLower(r.Value)
		switch r.Op {
		case "is":
			return lower == v
		case "startswith":
			return strings.HasPrefix(lower, v)
		case "contains":
			return strings.Contains(lower, v)
		}
	case "size":
		want, err := strconv.ParseInt(r.Value, 10, 64)
		if err != nil {
			return false
		}
		switch r.Op {
		case "gt":
			return f.Size > want
		case "lt":
			return f.Size < want
		}
	}
	return false
}

// Presets are ready-made rule bundles offered in the UI.
var Presets = map[string][]Rule{
	"media_to_google": {
		{Priority: 1, Field: "mime", Op: "is", Value: "video/", Target: "", Enabled: true},
		{Priority: 2, Field: "mime", Op: "is", Value: "image/", Target: "", Enabled: true},
	},
	"docs_to_mybox": {
		{Priority: 1, Field: "mime", Op: "is", Value: "application/pdf", Target: "", Enabled: true},
		{Priority: 2, Field: "mime", Op: "is", Value: "text/", Target: "", Enabled: true},
	},
}
