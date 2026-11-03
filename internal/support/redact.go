package support

import "regexp"

const redactedSerial = "[serial redacted]"

// xochitl writes the tablet's serial into its log output; the two shapes cover the reMarkable 2 and the Paper Pro family.
var serialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bRM\d{3}-\d{3}-\d{5}\b`),
	regexp.MustCompile(`\bRM\d{2}A[0-9A-Z]{10}\b`),
}

// xochitl's per-tablet authorId is a name-based UUID, so it identifies the tablet as well.
var authorIDPattern = regexp.MustCompile(`(authorId(?: to |=|: ?))[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func redactDeviceIdentifiers(data []byte) []byte {
	for _, pattern := range serialPatterns {
		data = pattern.ReplaceAll(data, []byte(redactedSerial))
	}
	return authorIDPattern.ReplaceAll(data, []byte("${1}[author id redacted]"))
}
