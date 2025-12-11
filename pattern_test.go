package ragfs

import (
	"fmt"
	"testing"
)

func TestParsePattern_BasicPath(t *testing.T) {
	p, err := parsePattern("/users")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if p.str != "/users" {
		t.Errorf("expected str '/users', got %q", p.str)
	}
	if p.perm != 0777 {
		t.Errorf("expected default perm 0777, got %o", p.perm)
	}
	if len(p.segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.segments))
	}
	if p.segments[0].value != "users" {
		t.Errorf("expected segment 'users', got %q", p.segments[0].value)
	}
	if p.segments[0].wild {
		t.Error("expected segment to not be wild")
	}
}

func TestParsePattern_WithPermission(t *testing.T) {
	p, err := parsePattern("755 /users/{id}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if p.str != "755 /users/{id}" {
		t.Errorf("expected str '755 /users/{id}', got %q", p.str)
	}
	if p.perm != 0755 {
		t.Errorf("expected perm 0755, got %o", p.perm)
	}
	if len(p.segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(p.segments))
	}
	if p.segments[0].value != "users" || p.segments[0].wild {
		t.Errorf("expected first segment 'users' (not wild), got %q (wild=%v)", p.segments[0].value, p.segments[0].wild)
	}
	if p.segments[1].value != "id" || !p.segments[1].wild {
		t.Errorf("expected second segment 'id' (wild), got %q (wild=%v)", p.segments[1].value, p.segments[1].wild)
	}
}

func TestParsePattern_WithWildcard(t *testing.T) {
	p, err := parsePattern("/files/{name}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if len(p.segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(p.segments))
	}
	if !p.segments[1].wild {
		t.Error("expected second segment to be wild")
	}
	if p.segments[1].multi {
		t.Error("expected second segment to not be multi")
	}
	if p.segments[1].value != "name" {
		t.Errorf("expected wildcard name 'name', got %q", p.segments[1].value)
	}
}

func TestParsePattern_WithRestWildcard(t *testing.T) {
	p, err := parsePattern("/files/{path...}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if len(p.segments) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(p.segments))
	}
	if !p.segments[1].wild {
		t.Error("expected second segment to be wild")
	}
	if !p.segments[1].multi {
		t.Error("expected second segment to be multi (rest wildcard)")
	}
	if p.segments[1].value != "path" {
		t.Errorf("expected wildcard name 'path', got %q", p.segments[1].value)
	}
}

func TestParsePattern_ExactMatch(t *testing.T) {
	p, err := parsePattern("/{$}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if !p.isExact {
		t.Error("expected pattern to be exact match")
	}
	if len(p.segments) != 0 {
		t.Errorf("expected 0 segments for exact root match, got %d", len(p.segments))
	}
}

func TestParsePattern_ExactMatchWithPath(t *testing.T) {
	p, err := parsePattern("/users/{$}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if !p.isExact {
		t.Error("expected pattern to be exact match")
	}
	if len(p.segments) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(p.segments))
	}
	if p.segments[0].value != "users" {
		t.Errorf("expected segment 'users', got %q", p.segments[0].value)
	}
}

func TestParsePattern_TrailingSlash(t *testing.T) {
	p, err := parsePattern("/users/")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if len(p.segments) != 2 {
		t.Fatalf("expected 2 segments (including anonymous multi), got %d", len(p.segments))
	}
	if !p.segments[1].multi {
		t.Error("trailing slash should create anonymous multi wildcard")
	}
}

func TestParsePattern_Invalid(t *testing.T) {
	testCases := []struct {
		name    string
		pattern string
	}{
		{"emptyPattern", ""},
		{"noSlash", "users"},
		{"invalidPermission", "999 /users"},
		{"invalidWildcardMiddle", "/users/{path...}/files"},
		{"unclosedBrace", "/users/{id"},
		{"emptyWildcard", "/users/{}"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePattern(tc.pattern)
			if err == nil {
				t.Errorf("expected error for pattern %q, got nil", tc.pattern)
			}
		})
	}
}

func TestParsePattern_MultipleWildcards(t *testing.T) {
	p, err := parsePattern("/users/{id}/files/{name}")
	if err != nil {
		t.Fatalf("parsePattern returned error: %v", err)
	}
	if len(p.segments) != 4 {
		t.Fatalf("expected 4 segments, got %d", len(p.segments))
	}
	if p.segments[0].value != "users" || p.segments[0].wild {
		t.Errorf("expected segment 0 'users' (not wild)")
	}
	if p.segments[1].value != "id" || !p.segments[1].wild {
		t.Errorf("expected segment 1 'id' (wild)")
	}
	if p.segments[2].value != "files" || p.segments[2].wild {
		t.Errorf("expected segment 2 'files' (not wild)")
	}
	if p.segments[3].value != "name" || !p.segments[3].wild {
		t.Errorf("expected segment 3 'name' (wild)")
	}
}

func TestParsePattern_PermissionVariants(t *testing.T) {
	testCases := []struct {
		pattern string
		perm    uint16
	}{
		{"777 /public", 0777},
		{"644 /config", 0644},
		{"400 /secret", 0400},
		{"000 /hidden", 0000},
		{"/default", 0777}, // default when no permission specified
	}

	for _, tc := range testCases {
		t.Run(tc.pattern, func(t *testing.T) {
			p, err := parsePattern(tc.pattern)
			if err != nil {
				t.Fatalf("parsePattern returned error: %v", err)
			}
			if p.perm != tc.perm {
				t.Errorf("expected perm %o, got %o", tc.perm, p.perm)
			}
		})
	}
}

// Phase 2: Path Sanitization Tests

func TestCleanPath(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"/a/b/c", "/a/b/c"},
		{"/a/../b", "/b"},
		{"/a/./b", "/a/b"},
		{"/a//b", "/a/b"},
		{"/../a", "/a"},
		{"/a/b/../c", "/a/c"},
		{"/a/b/./c", "/a/b/c"},
		{"//a//b//", "/a/b"},
		{"/", "/"},
		{"", "/"},
		{"a/b", "/a/b"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			got := cleanPath(tc.input)
			if got != tc.expected {
				t.Errorf("cleanPath(%q) = %q, expected %q", tc.input, got, tc.expected)
			}
		})
	}
}

// Phase 3: Path Matching Tests

func TestMatch_StaticPath(t *testing.T) {
	p, _ := parsePattern("/users")

	params, ok := p.match("/users")
	if !ok {
		t.Error("expected match")
	}
	if len(params) != 0 {
		t.Errorf("expected no params, got %v", params)
	}

	_, ok = p.match("/other")
	if ok {
		t.Error("expected no match for /other")
	}

	_, ok = p.match("/users/123")
	if ok {
		t.Error("expected no match for /users/123")
	}
}

func TestMatch_SingleWildcard(t *testing.T) {
	p, _ := parsePattern("/users/{id}")

	params, ok := p.match("/users/123")
	if !ok {
		t.Error("expected match")
	}
	if params["id"] != "123" {
		t.Errorf("expected id='123', got %q", params["id"])
	}

	_, ok = p.match("/users")
	if ok {
		t.Error("expected no match for /users")
	}

	_, ok = p.match("/users/123/extra")
	if ok {
		t.Error("expected no match for /users/123/extra")
	}
}

func TestMatch_RestWildcard(t *testing.T) {
	p, _ := parsePattern("/files/{path...}")

	params, ok := p.match("/files/a/b/c")
	if !ok {
		t.Error("expected match")
	}
	if params["path"] != "a/b/c" {
		t.Errorf("expected path='a/b/c', got %q", params["path"])
	}

	params, ok = p.match("/files/single")
	if !ok {
		t.Error("expected match for single segment")
	}
	if params["path"] != "single" {
		t.Errorf("expected path='single', got %q", params["path"])
	}

	_, ok = p.match("/files")
	if ok {
		t.Error("expected no match for /files without path")
	}
}

func TestMatch_ExactMatch(t *testing.T) {
	p, _ := parsePattern("/{$}")

	_, ok := p.match("/")
	if !ok {
		t.Error("expected match for /")
	}

	_, ok = p.match("/anything")
	if ok {
		t.Error("expected no match for /anything with exact match")
	}
}

func TestMatch_ExactMatchWithPath(t *testing.T) {
	p, _ := parsePattern("/users/{$}")

	_, ok := p.match("/users")
	if !ok {
		t.Error("expected match for /users")
	}

	_, ok = p.match("/users/")
	if ok {
		t.Error("expected no match for /users/ with exact match")
	}

	_, ok = p.match("/users/123")
	if ok {
		t.Error("expected no match for /users/123 with exact match")
	}
}

func TestMatch_TrailingSlash(t *testing.T) {
	p, _ := parsePattern("/users/")

	_, ok := p.match("/users/")
	if !ok {
		t.Error("expected match for /users/")
	}

	_, ok = p.match("/users/123")
	if !ok {
		t.Error("expected match for /users/123")
	}

	_, ok = p.match("/users/a/b/c")
	if !ok {
		t.Error("expected match for /users/a/b/c")
	}
}

func TestMatch_MultipleWildcards(t *testing.T) {
	p, _ := parsePattern("/users/{id}/files/{name}")

	params, ok := p.match("/users/123/files/readme.txt")
	if !ok {
		t.Error("expected match")
	}
	if params["id"] != "123" {
		t.Errorf("expected id='123', got %q", params["id"])
	}
	if params["name"] != "readme.txt" {
		t.Errorf("expected name='readme.txt', got %q", params["name"])
	}
}

func TestMatch_RootPath(t *testing.T) {
	p, _ := parsePattern("/")

	_, ok := p.match("/")
	if !ok {
		t.Error("expected match for /")
	}

	_, ok = p.match("/anything")
	if !ok {
		t.Error("/ should match /anything (subtree pattern)")
	}
}

// Phase 4: Specificity Comparison Tests

func TestMoreSpecificThan_StaticBeatsWildcard(t *testing.T) {
	p1, _ := parsePattern("/users/123")
	p2, _ := parsePattern("/users/{id}")

	if !p1.moreSpecificThan(p2) {
		t.Error("static path should be more specific than wildcard")
	}
	if p2.moreSpecificThan(p1) {
		t.Error("wildcard should not be more specific than static")
	}
}

func TestMoreSpecificThan_LongerPrefixWins(t *testing.T) {
	p1, _ := parsePattern("/users/{id}")
	p2, _ := parsePattern("/")

	if !p1.moreSpecificThan(p2) {
		t.Error("longer path should be more specific than root")
	}
}

func TestMoreSpecificThan_ExactBeatsSubtree(t *testing.T) {
	p1, _ := parsePattern("/users/{$}")
	p2, _ := parsePattern("/users/")

	if !p1.moreSpecificThan(p2) {
		t.Error("exact match should be more specific than subtree")
	}
	if p2.moreSpecificThan(p1) {
		t.Error("subtree should not be more specific than exact")
	}
}

func TestMoreSpecificThan_SingleWildcardBeatsMulti(t *testing.T) {
	p1, _ := parsePattern("/files/{name}")
	p2, _ := parsePattern("/files/{path...}")

	if !p1.moreSpecificThan(p2) {
		t.Error("single wildcard should be more specific than multi")
	}
}

func TestConflictsWith(t *testing.T) {
	testCases := []struct {
		name     string
		pattern1 string
		pattern2 string
		conflict bool
	}{
		{"samePattern", "/users/{id}", "/users/{name}", true},
		{"differentPath", "/users/{id}", "/files/{id}", false},
		{"staticVsWildcard", "/users/123", "/users/{id}", false},
		{"exactVsSubtree", "/users/{$}", "/users/", false},
		{"identicalStatic", "/users", "/users", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p1, _ := parsePattern(tc.pattern1)
			p2, _ := parsePattern(tc.pattern2)

			got := p1.conflictsWith(p2)
			if got != tc.conflict {
				t.Errorf("conflictsWith(%q, %q) = %v, expected %v", tc.pattern1, tc.pattern2, got, tc.conflict)
			}
		})
	}
}

// Phase 5: Permission Checking Tests

func TestCountPermBits(t *testing.T) {
	testCases := []struct {
		perm     uint16
		expected int
	}{
		{0777, 9}, // all bits set
		{0755, 7}, // rwxr-xr-x = 7+5+5 in binary = 111 101 101 = 7 bits
		{0644, 4}, // rw-r--r-- = 110 100 100 = 4 bits
		{0400, 1}, // r-------- = 100 000 000 = 1 bit
		{0000, 0}, // --------- = 0 bits
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%03o", tc.perm), func(t *testing.T) {
			got := countPermBits(tc.perm)
			if got != tc.expected {
				t.Errorf("countPermBits(%o) = %d, expected %d", tc.perm, got, tc.expected)
			}
		})
	}
}

func TestAllowsOpen(t *testing.T) {
	testCases := []struct {
		pattern  string
		expected bool
	}{
		{"755 /users", true},   // owner has read (7 = rwx, includes r)
		{"644 /config", true},  // owner has read (6 = rw-)
		{"400 /secret", true},  // owner has read (4 = r--)
		{"311 /exec", false},   // owner has no read (3 = -wx)
		{"111 /run", false},    // owner has no read (1 = --x)
		{"000 /hidden", false}, // no permissions
		{"/default", true},     // default 777 has read
	}

	for _, tc := range testCases {
		t.Run(tc.pattern, func(t *testing.T) {
			p, err := parsePattern(tc.pattern)
			if err != nil {
				t.Fatalf("parsePattern error: %v", err)
			}
			got := p.allowsOpen()
			if got != tc.expected {
				t.Errorf("allowsOpen() = %v, expected %v (perm=%o)", got, tc.expected, p.perm)
			}
		})
	}
}

func TestAllowsReadDir(t *testing.T) {
	testCases := []struct {
		pattern  string
		expected bool
	}{
		{"755 /users", true},    // owner has execute (7 = rwx, includes x)
		{"644 /config", false},  // owner has no execute (6 = rw-)
		{"500 /secret", true},   // owner has execute (5 = r-x)
		{"311 /exec", true},     // owner has execute (3 = -wx)
		{"111 /run", true},      // owner has execute (1 = --x)
		{"000 /hidden", false},  // no permissions
		{"600 /private", false}, // owner has no execute (6 = rw-)
		{"/default", true},      // default 777 has execute
	}

	for _, tc := range testCases {
		t.Run(tc.pattern, func(t *testing.T) {
			p, err := parsePattern(tc.pattern)
			if err != nil {
				t.Fatalf("parsePattern error: %v", err)
			}
			got := p.allowsReadDir()
			if got != tc.expected {
				t.Errorf("allowsReadDir() = %v, expected %v (perm=%o)", got, tc.expected, p.perm)
			}
		})
	}
}

func TestPermissionTieBreaker(t *testing.T) {
	p1, _ := parsePattern("644 /users/{id}")
	p2, _ := parsePattern("755 /users/{id}")

	// Both match same paths, but 644 is more restrictive (4 bits vs 7 bits)
	// So p1 should "win" as a tie-breaker

	bits1 := countPermBits(p1.perm)
	bits2 := countPermBits(p2.perm)

	if bits1 >= bits2 {
		t.Errorf("644 should have fewer bits than 755: %d vs %d", bits1, bits2)
	}
}

// Phase 6: Integration Tests (using internal types)

func TestSelectBestMatch_MostSpecific(t *testing.T) {
	routes := []routeInternal{
		{pattern: mustParse("/users/")},
		{pattern: mustParse("/users/{id}")},
		{pattern: mustParse("/users/123")},
	}

	best := selectBestMatch(routes, "/users/123", true)
	if best == nil {
		t.Fatal("expected match")
	}
	if best.pattern.str != "/users/123" {
		t.Errorf("expected /users/123 to match, got %s", best.pattern.str)
	}
}

func TestSelectBestMatch_PermissionFiltering(t *testing.T) {
	routes := []routeInternal{
		{pattern: mustParse("400 /secret")},
	}

	// 400 has read permission, so Open should work
	best := selectBestMatch(routes, "/secret", true)
	if best == nil {
		t.Error("expected match for Open with read permission")
	}

	// 400 has no execute permission, so ReadDir should not match
	best = selectBestMatch(routes, "/secret", false)
	if best != nil {
		t.Error("expected no match for ReadDir without execute permission")
	}
}

func TestSelectBestMatch_PermissionTieBreaker(t *testing.T) {
	routes := []routeInternal{
		{pattern: mustParse("755 /files/{name}")},
		{pattern: mustParse("644 /files/{name}")},
	}

	// Both patterns match the same paths, 644 is more restrictive
	best := selectBestMatch(routes, "/files/test.txt", true)
	if best == nil {
		t.Fatal("expected match")
	}
	// 644 should win because it has fewer permission bits
	if best.pattern.perm != 0644 {
		t.Errorf("expected 644 to win, got %o", best.pattern.perm)
	}
}

func mustParse(s string) *pattern {
	p, err := parsePattern(s)
	if err != nil {
		panic(err)
	}
	return p
}
