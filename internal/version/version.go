package version

import "strings"

// Suffix ranking from apk's version rules: below noSuffix is a pre-release sorting under the plain version, above is built on top, so 1.0_pre1 < 1.0 < 1.0_git1.
var suffixRank = map[string]int{
	"alpha": 0,
	"beta":  1,
	"pre":   2,
	"rc":    3,
	"cvs":   5,
	"svn":   6,
	"git":   7,
	"hg":    8,
	"p":     9,
}

const noSuffix = 4

type parsed struct {
	numbers  []int
	letter   byte
	suffixes []suffix
	revision int
}

type suffix struct {
	rank   int
	number int
}

// Compare orders two versions by apk's rules (-1, 0, 1); a numeric split alone reads 0.0.11_pre1 as 0.0.11 and leaves prereleases in arrival order.
func Compare(a, b string) int {
	pa := parse(strings.TrimPrefix(a, "v"))
	pb := parse(strings.TrimPrefix(b, "v"))

	maxLen := len(pa.numbers)
	if len(pb.numbers) > maxLen {
		maxLen = len(pb.numbers)
	}
	for i := 0; i < maxLen; i++ {
		var an, bn int
		if i < len(pa.numbers) {
			an = pa.numbers[i]
		}
		if i < len(pb.numbers) {
			bn = pb.numbers[i]
		}
		if c := sign(an - bn); c != 0 {
			return c
		}
	}

	if c := sign(int(pa.letter) - int(pb.letter)); c != 0 {
		return c
	}

	maxLen = len(pa.suffixes)
	if len(pb.suffixes) > maxLen {
		maxLen = len(pb.suffixes)
	}
	for i := 0; i < maxLen; i++ {
		as := suffix{rank: noSuffix}
		bs := suffix{rank: noSuffix}
		if i < len(pa.suffixes) {
			as = pa.suffixes[i]
		}
		if i < len(pb.suffixes) {
			bs = pb.suffixes[i]
		}
		if c := sign(as.rank - bs.rank); c != 0 {
			return c
		}
		if c := sign(as.number - bs.number); c != 0 {
			return c
		}
	}

	return sign(pa.revision - pb.revision)
}

func parse(v string) parsed {
	var p parsed

	if base, rev, found := strings.Cut(v, "-r"); found {
		v = base
		p.revision = atoi(rev)
	} else {
		// A dash that is not -rN is outside apk's grammar, but callers pass dotted-and-dashed strings, so keep the tail rather than drop it.
		v = strings.ReplaceAll(v, "-", ".")
	}

	head := v
	if i := strings.IndexByte(v, '_'); i >= 0 {
		head = v[:i]
		for _, part := range strings.Split(v[i+1:], "_") {
			name := strings.TrimRight(part, "0123456789")
			rank, ok := suffixRank[name]
			if !ok {
				rank = noSuffix
			}
			p.suffixes = append(p.suffixes, suffix{rank: rank, number: atoi(part[len(name):])})
		}
	}

	for _, part := range strings.Split(head, ".") {
		digits := strings.TrimRight(part, "abcdefghijklmnopqrstuvwxyz")
		p.numbers = append(p.numbers, atoi(digits))
		if len(digits) < len(part) {
			p.letter = part[len(digits)]
		}
	}

	return p
}

func atoi(s string) int {
	num := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		num = num*10 + int(c-'0')
	}
	return num
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	}
	return 0
}
