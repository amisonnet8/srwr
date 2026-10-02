package tape

import "strings"

// FileSuffix ends the name of every tape file; what comes before it is the tape ID.
const FileSuffix = ".tape.jsonl"

// FileName returns the file name of a tape.
func FileName(id string) string { return id + FileSuffix }

// ValidID reports whether id is a bare tape ID such as "20261001-1706-1795": no path separators,
// nothing that could point outside the tapes directory. A tape ID read from a file or a request
// (.srwr/active, a shared tape) must pass this before it is used to build a path.
func ValidID(id string) bool {
	if id == "" || strings.HasPrefix(id, ".") {
		return false
	}
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}
