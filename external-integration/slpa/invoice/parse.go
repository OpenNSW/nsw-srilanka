package invoice

import (
	"strconv"
	"strings"
)

// parseFloat reads an amount the CMS sent as a string. Their figures carry
// thousands separators in some places ("4,776"), which strconv will not take.
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
}
