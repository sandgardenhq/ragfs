package ragfs

import (
	"errors"
	"math/bits"
	"path"
	"strconv"
	"strings"
)

type segment struct {
	value string
	wild  bool
	multi bool
}

type pattern struct {
	str      string
	perm     uint16
	segments []segment
	isExact  bool
}

func parsePattern(s string) (*pattern, error) {
	if s == "" {
		return nil, errors.New("empty pattern")
	}

	p := &pattern{
		str:  s,
		perm: 0777,
	}

	remaining := s

	// Check for permission prefix (e.g., "755 /users")
	if len(remaining) >= 4 && remaining[3] == ' ' {
		permStr := remaining[:3]
		perm, err := strconv.ParseInt(permStr, 8, 16)
		if err != nil || perm < 0 || perm > 0777 {
			return nil, errors.New("invalid permission: " + permStr)
		}
		p.perm = uint16(perm)
		remaining = remaining[4:]
	}

	// Path must start with /
	if !strings.HasPrefix(remaining, "/") {
		return nil, errors.New("path must start with /")
	}

	// Handle root path
	if remaining == "/" {
		return p, nil
	}

	// Remove leading slash for parsing
	remaining = remaining[1:]

	// Check for exact match marker at end
	if strings.HasSuffix(remaining, "{$}") {
		p.isExact = true
		remaining = strings.TrimSuffix(remaining, "{$}")
		remaining = strings.TrimSuffix(remaining, "/")
		if remaining == "" {
			return p, nil
		}
	}

	// Check for trailing slash (creates anonymous multi wildcard)
	hasTrailingSlash := strings.HasSuffix(remaining, "/")
	if hasTrailingSlash {
		remaining = strings.TrimSuffix(remaining, "/")
	}

	// Split into segments
	if remaining != "" {
		parts := strings.Split(remaining, "/")
		for i, part := range parts {
			seg, err := parseSegment(part)
			if err != nil {
				return nil, err
			}

			// Multi wildcard must be at end
			if seg.multi && i != len(parts)-1 {
				return nil, errors.New("rest wildcard {...} must be at end of pattern")
			}

			p.segments = append(p.segments, seg)
		}
	}

	// Trailing slash creates anonymous multi wildcard
	if hasTrailingSlash {
		p.segments = append(p.segments, segment{
			value: "",
			wild:  true,
			multi: true,
		})
	}

	return p, nil
}

func parseSegment(s string) (segment, error) {
	if s == "" {
		return segment{}, errors.New("empty segment")
	}

	// Check for wildcard pattern {name} or {name...}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") {
		name := s[1 : len(s)-1]
		if name == "" {
			return segment{}, errors.New("empty wildcard name")
		}

		// Check for rest wildcard
		if strings.HasSuffix(name, "...") {
			name = strings.TrimSuffix(name, "...")
			if name == "" {
				name = "_rest"
			}
			return segment{value: name, wild: true, multi: true}, nil
		}

		return segment{value: name, wild: true, multi: false}, nil
	}

	// Check for unclosed brace
	if strings.Contains(s, "{") || strings.Contains(s, "}") {
		return segment{}, errors.New("invalid wildcard syntax: " + s)
	}

	// Static segment
	return segment{value: s, wild: false, multi: false}, nil
}

func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if p[0] != '/' {
		p = "/" + p
	}
	return path.Clean(p)
}

func (p *pattern) match(pathStr string) (map[string]string, bool) {
	hasTrailingSlash := len(pathStr) > 1 && strings.HasSuffix(pathStr, "/")
	pathStr = cleanPath(pathStr)
	params := make(map[string]string)

	// For exact match patterns, reject paths with trailing slashes
	// unless the pattern itself is just the root
	if p.isExact && hasTrailingSlash {
		return nil, false
	}

	// Handle root path pattern
	if len(p.segments) == 0 {
		if p.isExact {
			return params, pathStr == "/"
		}
		return params, true
	}

	// Remove leading slash and split path
	pathStr = strings.TrimPrefix(pathStr, "/")
	var pathParts []string
	if pathStr != "" {
		pathParts = strings.Split(pathStr, "/")
	}

	segIdx := 0
	pathIdx := 0

	for segIdx < len(p.segments) {
		seg := p.segments[segIdx]

		if seg.multi {
			if pathIdx >= len(pathParts) {
				if seg.value == "" {
					return params, true
				}
				return nil, false
			}
			rest := strings.Join(pathParts[pathIdx:], "/")
			if seg.value != "" {
				params[seg.value] = rest
			}
			return params, true
		}

		if pathIdx >= len(pathParts) {
			return nil, false
		}

		if seg.wild {
			params[seg.value] = pathParts[pathIdx]
		} else {
			if pathParts[pathIdx] != seg.value {
				return nil, false
			}
		}

		segIdx++
		pathIdx++
	}

	if pathIdx < len(pathParts) {
		return nil, false
	}

	return params, true
}

func (p *pattern) moreSpecificThan(other *pattern) bool {
	if p.str == other.str {
		return false
	}

	pLen := len(p.segments)
	oLen := len(other.segments)

	pHasMulti := pLen > 0 && p.segments[pLen-1].multi
	oHasMulti := oLen > 0 && other.segments[oLen-1].multi

	pStaticLen := pLen
	if pHasMulti {
		pStaticLen--
	}
	oStaticLen := oLen
	if oHasMulti {
		oStaticLen--
	}

	if pLen == 0 && oLen > 0 {
		return false
	}
	if pLen > 0 && oLen == 0 {
		return true
	}

	minLen := min(pStaticLen, oStaticLen)

	for i := 0; i < minLen; i++ {
		ps := p.segments[i]
		os := other.segments[i]

		if ps.wild && !os.wild {
			return false
		}
		if !ps.wild && os.wild {
			return true
		}
	}

	if pStaticLen == oStaticLen {
		if p.isExact && !other.isExact {
			return true
		}
		if !p.isExact && other.isExact {
			return false
		}

		if !pHasMulti && oHasMulti {
			return true
		}
		if pHasMulti && !oHasMulti {
			return false
		}
	}

	if pStaticLen > oStaticLen && !p.isExact && oHasMulti {
		return true
	}

	if pStaticLen > oStaticLen && p.isExact {
		return true
	}

	if pStaticLen < oStaticLen {
		return false
	}

	return false
}

func (p *pattern) conflictsWith(other *pattern) bool {
	if p.str == other.str {
		return true
	}

	if p.moreSpecificThan(other) || other.moreSpecificThan(p) {
		return false
	}

	if len(p.segments) != len(other.segments) {
		return false
	}

	for i := range p.segments {
		ps := p.segments[i]
		os := other.segments[i]

		if ps.wild != os.wild {
			return false
		}
		if ps.multi != os.multi {
			return false
		}
		if !ps.wild && ps.value != os.value {
			return false
		}
	}

	return p.isExact == other.isExact
}

func countPermBits(perm uint16) int {
	return bits.OnesCount16(perm)
}

func (p *pattern) allowsOpen() bool {
	ownerPerm := (p.perm >> 6) & 0x7
	return (ownerPerm & 0x4) != 0
}

func (p *pattern) allowsReadDir() bool {
	ownerPerm := (p.perm >> 6) & 0x7
	return (ownerPerm & 0x1) != 0
}
