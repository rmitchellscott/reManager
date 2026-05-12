package version

import "strings"

func Compare(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	aParts := parseVersionParts(a)
	bParts := parseVersionParts(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			aNum = aParts[i]
		}
		if i < len(bParts) {
			bNum = bParts[i]
		}
		if aNum > bNum {
			return 1
		}
		if aNum < bNum {
			return -1
		}
	}
	return 0
}

func parseVersionParts(v string) []int {
	var parts []int
	var current string
	for _, c := range v {
		if c == '.' || c == '-' {
			if current != "" {
				parts = append(parts, parseNum(current))
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, parseNum(current))
	}
	return parts
}

func parseNum(s string) int {
	num := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
		} else {
			break
		}
	}
	return num
}
