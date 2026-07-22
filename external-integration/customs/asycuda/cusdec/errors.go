package cusdec

import "errors"

// ErrWorkflowNotFoundByEdgeID indicates no task workflow matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrWorkflowNotFoundByEdgeID = errors.New("workflow not found by edgeId")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")
