package consignment

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/OpenNSW/nsw-srilanka/internal/profile/company"
)

// Every key caseAttributes writes is shared: one that is not would never reach an agency.
func TestCaseAttributesAreShared(t *testing.T) {
	for key := range caseAttributes(&company.Record{Name: "Adam PVT LTD"}) {
		assert.True(t, slices.Contains(SharedCaseAttributes, key), "key %q is not in SharedCaseAttributes", key)
	}
}
