package cusdec

import "errors"

// ErrWorkflowNotFoundByEdgeID indicates no task workflow matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrWorkflowNotFoundByEdgeID = errors.New("workflow not found by edgeId")

// ErrCusdecNotFoundByRef indicates no Customs Declaration matches the given reference.
// This can be a transient condition if an event callback arrives before the integration
// result callback has finished processing — the caller should retry.
var ErrCusdecNotFoundByRef = errors.New("customs declaration not found by reference")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")
