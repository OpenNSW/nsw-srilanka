package cdn

import "errors"

// ErrDispatchNoteNotFoundByEdgeID indicates no dispatch note matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrDispatchNoteNotFoundByEdgeID = errors.New("dispatch note not found by edgeId")

// ErrDispatchNoteNotFoundByCDNRef indicates no dispatch note matches the given cdnRef.
// This can be a transient condition if the acknowledgment callback arrives before
// the integration result callback has finished processing — the caller should retry.
var ErrDispatchNoteNotFoundByCDNRef = errors.New("dispatch note not found by cdnRef")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")
