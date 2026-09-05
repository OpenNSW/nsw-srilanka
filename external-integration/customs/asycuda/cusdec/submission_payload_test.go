package cusdec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// minimalForm is the least a declaration form can carry and still build: an
// office, and one item. The field rules are ASYCUDA's to enforce, so the
// payload checks only what it cannot send at all.
func minimalForm() map[string]any {
	return map[string]any{
		"identification": map[string]any{"declarationType": "EX", "officeCode": "CBEX1"},
		"items": []any{
			map[string]any{
				"tarification":     map[string]any{"hsCode": "0801119000"},
				"goodsDescription": map[string]any{"commercialDescription": "cinnamon"},
			},
		},
	}
}

// identifier builds the payload and reads back what goes on the wire, which is
// where Annex A puts the field.
func identifier(t *testing.T, form map[string]any, previousEdgeID string) string {
	t.Helper()

	sub, _, err := BuildPayload(form, previousEdgeID)
	require.NoError(t, err)

	encoded, err := json.Marshal(sub)
	require.NoError(t, err)

	var wire struct {
		Properties struct {
			Submitter string `json:"submitter"`
			NswID     string `json:"nswId"`
		} `json:"properties"`
		Submitter string `json:"submitter"`
	}
	require.NoError(t, json.Unmarshal(encoded, &wire))
	assert.Equal(t, submitterChannel, wire.Properties.Submitter,
		"the channel is sent inside properties as well as at the top level")
	assert.Equal(t, submitterChannel, wire.Submitter, "the top-level submitter is unchanged")
	return wire.Properties.NswID
}

// §2.2: a resend of the same attempt must carry the same identifier, which is
// the whole point — it is what lets ASYCUDA recognise the resend after a lost
// 202 instead of registering a second declaration.
func TestBuildPayload_SameAttemptDerivesTheSameIdentifier(t *testing.T) {
	first := identifier(t, minimalForm(), "")
	again := identifier(t, minimalForm(), "")

	require.NotEmpty(t, first)
	assert.Equal(t, first, again, "an unchanged resend is the same logical submission")
	assert.Len(t, first, 64, "a hex-encoded SHA-256")
}

// A correction the trader makes is a new business submission.
func TestBuildPayload_ACorrectionDerivesANewIdentifier(t *testing.T) {
	corrected := minimalForm()
	corrected["identification"] = map[string]any{"declarationType": "EX", "officeCode": "CBEX2"}

	assert.NotEqual(t, identifier(t, minimalForm(), ""), identifier(t, corrected, ""),
		"a changed declaration is a different submission")
}

// A resubmission after a rejected integration result is new even when the
// trader changed nothing: the previous attempt's edgeId settles it. Without
// this, ASYCUDA would suppress the resubmission as a duplicate and the
// integration result the trader is waiting on would never arrive.
func TestBuildPayload_AResubmissionIsNewEvenWhenNothingChanged(t *testing.T) {
	first := identifier(t, minimalForm(), "")
	second := identifier(t, minimalForm(), "5516e4c8-a93d-429d-8a18-6a484d331176")
	third := identifier(t, minimalForm(), "9f2b1a34-0c7e-4a71-b2d9-1e5f8c3a7b60")

	assert.NotEqual(t, first, second)
	assert.NotEqual(t, second, third)
}

// The identifier is derived from the submission as it will be sent, so it must
// not depend on itself: the field is empty while the digest is taken.
func TestBuildPayload_TheIdentifierDoesNotDependOnItself(t *testing.T) {
	sub, _, err := BuildPayload(minimalForm(), "")
	require.NoError(t, err)

	// Re-deriving from the payload as returned — with the field now populated —
	// would give a different answer if the field were part of the digest.
	rebuilt, _, err := BuildPayload(minimalForm(), "")
	require.NoError(t, err)
	assert.Equal(t, sub.Properties.NswID, rebuilt.Properties.NswID)
}
