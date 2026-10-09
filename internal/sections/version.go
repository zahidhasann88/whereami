package sections

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	versionRe = regexp.MustCompile(`\d+(?:\.\d+)*`)
	opRe      = regexp.MustCompile(`(>=|<=|==|!=|~=|>|<|=|\^|~)\s+`)
)

// parseVersion extracts the first numeric version from command output.
func parseVersion(s string) ([]int, bool) {
	m := versionRe.FindString(s)
	if m == "" {
		return nil, false
	}
	return parseParts(strings.Split(m, "."))
}

func parseParts(fields []string) ([]int, bool) {
	out := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// compareVersions compares two numeric versions, treating missing components as zero.
func compareVersions(a, b []int) int {
	n := len(a)
	if len(b) > n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		var x, y int
		if i < len(a) {
			x = a[i]
		}
		if i < len(b) {
			y = b[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// satisfies supports numeric pins, wildcards, comparators, caret, tilde and AND/OR ranges.
// Unsupported syntax returns known=false rather than a mismatch.
func satisfies(v []int, expr string) (ok, known bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "*" || expr == "x" || expr == "X" {
		return true, true
	}
	expr = opRe.ReplaceAllString(expr, "$1")
	matched := false
	for _, alt := range strings.Split(expr, "||") {
		terms := strings.FieldsFunc(alt, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
		if len(terms) == 0 {
			return false, false
		}
		altOK := true
		for _, t := range terms {
			tOK, tKnown := satisfiesTerm(v, t)
			if !tKnown {
				return false, false
			}
			if !tOK {
				altOK = false
			}
		}
		if altOK {
			matched = true
		}
	}
	return matched, true
}

func satisfiesTerm(v []int, term string) (ok, known bool) {
	op := ""
	for _, candidate := range []string{">=", "<=", "==", "!=", "~=", ">", "<", "=", "^", "~"} {
		if strings.HasPrefix(term, candidate) {
			op = candidate
			term = term[len(candidate):]
			break
		}
	}
	term = strings.TrimPrefix(term, "v")
	parts, wildcard, ok := parseConstraintVersion(term)
	if !ok {
		return false, false
	}
	switch op {
	case "", "=", "==":
		if wildcard || len(parts) < 3 {
			return prefixMatch(v, parts), true
		}
		return compareVersions(v, parts) == 0, true
	case "!=":
		return compareVersions(v, parts) != 0, true
	case ">=":
		return compareVersions(v, parts) >= 0, true
	case ">":
		return compareVersions(v, parts) > 0, true
	case "<=":
		return compareVersions(v, parts) <= 0, true
	case "<":
		return compareVersions(v, parts) < 0, true
	case "~=":
		if len(parts) < 2 {
			return false, false
		}
		return compareVersions(v, parts) >= 0 && prefixMatch(v, parts[:len(parts)-1]), true
	case "^":
		idx := len(parts) - 1
		for i, p := range parts {
			if p != 0 {
				idx = i
				break
			}
		}
		return compareVersions(v, parts) >= 0 && compareVersions(v, bumpAt(parts, idx)) < 0, true
	case "~":
		idx := 0
		if len(parts) >= 2 {
			idx = 1
		}
		return compareVersions(v, parts) >= 0 && compareVersions(v, bumpAt(parts, idx)) < 0, true
	}
	return false, false
}

// parseConstraintVersion parses "1.2.3", "1.2", "1", "1.x" or "1.*".
func parseConstraintVersion(s string) (parts []int, wildcard, ok bool) {
	if s == "" {
		return nil, false, false
	}
	var fields []string
	for _, f := range strings.Split(s, ".") {
		if f == "x" || f == "X" || f == "*" {
			wildcard = true
			break
		}
		fields = append(fields, f)
	}
	if len(fields) == 0 {
		return nil, false, false
	}
	parts, ok = parseParts(fields)
	return parts, wildcard, ok
}

// prefixMatch reports whether v starts with the components of prefix.
func prefixMatch(v, prefix []int) bool {
	if len(v) < len(prefix) {
		return false
	}
	for i := range prefix {
		if v[i] != prefix[i] {
			return false
		}
	}
	return true
}

// bumpAt returns parts truncated after idx with component idx incremented.
func bumpAt(parts []int, idx int) []int {
	out := make([]int, idx+1)
	copy(out, parts[:idx+1])
	out[idx]++
	return out
}
