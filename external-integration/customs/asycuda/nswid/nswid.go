// Package nswid derives the submission identifier ASYCUDA reads as nswId.
package nswid

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// For returns the identifier for one logical submission (interface spec §2.2).
//
// SLC Edge's edgeId correlates the round trip once a submission has been
// acknowledged, but cannot close the case where the 202 itself is lost: NSW
// holds no edgeId, cannot tell that from a submission that never arrived, and
// resends. With nothing of NSW's own on the message, ASYCUDA registers a second
// declaration.
//
// So the value must be the same on every retry of one submission and different
// for the next. It is derived rather than minted, from the two things that
// settle which submission this is:
//
//   - the payload, so a correction the trader makes is a new submission; and
//   - the edgeId of this task's previous attempt, so a resubmission is new even
//     when the trader changed nothing — otherwise ASYCUDA would suppress it as
//     a duplicate and the integration result they are waiting for would never
//     come.
//
// Deriving rather than minting is what covers the lost-202 case. A resend of
// the same attempt — the activity re-running after a crash, the transport
// retrying — sees the same payload and the same previous edgeId, because the
// lost response never recorded a new one, and so derives the same identifier.
// A value minted per attempt would differ on exactly the resend it exists to
// suppress.
//
// The result is the SHA-256 of the two, hex encoded. §2.2 leaves the format to
// NSW ("the field name and format would follow NSW's convention") and Annex A
// types the field as a plain string, so nothing here has to look like the UUID
// the specification's example happens to show.
//
// payload is marshalled to derive from what will actually be sent. Marshalling
// is deterministic for a struct — field order is the struct's — so equal
// submissions derive equal identifiers. A payload that cannot be marshalled
// yields "", and the caller sends without the field rather than refusing a
// submission the trader would have no way to make.
func For(payload any, previousEdgeID string) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}

	sum := sha256.New()
	sum.Write(encoded)
	// Written as its own field rather than appended to the JSON, so an edgeId
	// cannot combine with a payload to produce the bytes of a different pair.
	sum.Write([]byte{0})
	sum.Write([]byte(previousEdgeID))

	return hex.EncodeToString(sum.Sum(nil))
}
