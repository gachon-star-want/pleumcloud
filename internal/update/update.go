// Package update answers "is there a newer release on GitHub?" for the
// manual-update banner (D13). The check sends no user data anywhere — a
// plain GET against the public releases API, cached for an hour.
package update

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultAPIBase is the public GitHub API; tests point it at httptest.
	DefaultAPIBase = "https://api.github.com"
	// Repo is the release source for the banner.
	Repo       = "gachon-star-want/pleumcloud"
	defaultTTL = time.Hour
)

// Result is what the banner renders.
type Result struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url,omitempty"`
}

type release struct {
	TagName string `json:"tag_name"`
	URL     string `json:"html_url"`
}

// Checker compares a running version against the latest GitHub release,
// caching the fetched release so a long-lived process doesn't hammer the
// API on every page load.
type Checker struct {
	APIBase string        // default DefaultAPIBase
	TTL     time.Duration // cache window, default 1h

	mu        sync.Mutex
	fetchedAt time.Time
	cached    release
}

// Check fetches (or reuses) the latest release and compares it against
// current. Network errors, unparsable tags and non-newer versions all
// collapse to Available=false so the UI can render blindly. Failed
// fetches are not cached — the next check retries.
func (c *Checker) Check(ctx context.Context, current string) Result {
	res := Result{Current: current}
	tag, url := c.latest(ctx)
	if tag == "" {
		return res
	}
	res.Latest, res.URL = tag, url
	res.Available = IsNewer(current, tag)
	return res
}

func (c *Checker) latest(ctx context.Context) (tag, url string) {
	c.mu.Lock()
	if !c.fetchedAt.IsZero() && time.Since(c.fetchedAt) < c.ttl() {
		defer c.mu.Unlock()
		return c.cached.TagName, c.cached.URL
	}
	c.mu.Unlock()

	base := c.APIBase
	if base == "" {
		base = DefaultAPIBase
	}
	fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(fctx, http.MethodGet,
		base+"/repos/"+Repo+"/releases/latest", nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", ""
	}
	var rel release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil || rel.TagName == "" {
		return "", ""
	}

	c.mu.Lock()
	c.cached, c.fetchedAt = rel, time.Now()
	c.mu.Unlock()
	return rel.TagName, rel.URL
}

func (c *Checker) ttl() time.Duration {
	if c.TTL > 0 {
		return c.TTL
	}
	return defaultTTL
}

// IsNewer reports whether candidate is a strictly newer semver than
// current. A leading "v" and build metadata are tolerated; anything that
// fails to parse yields false, so dev builds never nag.
func IsNewer(current, candidate string) bool {
	cur, ok := parseSemver(current)
	if !ok {
		return false
	}
	cand, ok := parseSemver(candidate)
	if !ok {
		return false
	}
	switch {
	case cur.major != cand.major:
		return cand.major > cur.major
	case cur.minor != cand.minor:
		return cand.minor > cur.minor
	case cur.patch != cand.patch:
		return cand.patch > cur.patch
	}
	// Same x.y.z: a release outranks any prerelease; otherwise the
	// prerelease identifiers decide (semver §11).
	if cand.pre == "" {
		return cur.pre != ""
	}
	if cur.pre == "" {
		return false
	}
	return preNewer(cur.pre, cand.pre)
}

type semver struct {
	major, minor, patch int
	pre                 string
}

func parseSemver(v string) (semver, bool) {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i] // build metadata never affects precedence
	}
	var pre string
	if i := strings.IndexByte(v, '-'); i >= 0 {
		pre, v = v[i+1:], v[:i]
		if !validPre(pre) {
			return semver{}, false
		}
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return semver{}, false
	}
	var n [3]int
	for i, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 || strconv.Itoa(x) != p { // rejects "01", "+1", ""
			return semver{}, false
		}
		n[i] = x
	}
	return semver{n[0], n[1], n[2], pre}, true
}

// validPre enforces semver prerelease identifiers: dot-separated, each
// non-empty and alnum/hyphen only; numeric identifiers carry no leading
// zeros.
func validPre(pre string) bool {
	if pre == "" {
		return false
	}
	for _, id := range strings.Split(pre, ".") {
		if id == "" {
			return false
		}
		allDigits := true
		for _, r := range id {
			if r < '0' || r > '9' {
				allDigits = false
				if !('a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || r == '-') {
					return false
				}
			}
		}
		if allDigits && len(id) > 1 && id[0] == '0' {
			return false
		}
	}
	return true
}

// preNewer compares prerelease strings per semver §11: identifier by
// identifier — numerics numerically, numerics below alphanumerics,
// alphanumerics lexically — and a longer identifier list outranks a
// shorter equal prefix.
func preNewer(cur, cand string) bool {
	curIDs := strings.Split(cur, ".")
	candIDs := strings.Split(cand, ".")
	for i := 0; i < len(curIDs) && i < len(candIDs); i++ {
		a, b := curIDs[i], candIDs[i]
		an, aerr := strconv.Atoi(a)
		bn, berr := strconv.Atoi(b)
		switch {
		case aerr == nil && berr == nil:
			if an != bn {
				return bn > an
			}
		case aerr == nil:
			return false // numeric < alphanumeric
		case berr == nil:
			return true
		default:
			if a != b {
				return a < b
			}
		}
	}
	return len(candIDs) > len(curIDs)
}
