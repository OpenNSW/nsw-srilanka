package cdn

import "errors"

// ErrDispatchNoteNotFoundByEdgeID indicates no dispatch note matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrDispatchNoteNotFoundByEdgeID = errors.New("dispatch note not found by edgeId")

// ErrDispatchNoteNotRecorded indicates an integration result arrived for an
// edgeId the dispatch step has not recorded yet. Transient: the step records it
// as soon as SLC Edge accepts the submission, so the caller should retry.
var ErrDispatchNoteNotRecorded = errors.New("no dispatch note recorded for edgeId")

// ErrDispatchNoteNotFoundByCDNRef indicates no dispatch note matches the given cdnRef.
// This can be a transient condition if the acknowledgment callback arrives before
// the integration result callback has finished processing — the caller should retry.
var ErrDispatchNoteNotFoundByCDNRef = errors.New("dispatch note not found by cdnRef")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")

// ErrTaskNotParked indicates a callback arrived for a dispatch note whose
// workflow has not yet parked the task it answers. Transient: answered with a
// 503, so SLC Edge retries (§2) once the task is waiting.
var ErrTaskNotParked = errors.New("dispatch note task not parked yet")

// ErrCaseNotRecorded indicates the dispatch note a notification names was never
// linked to the case it was dispatched from, so nothing can be opened on that
// case for it. Permanent: a retry finds the same row.
var ErrCaseNotRecorded = errors.New("dispatch note has no case recorded")
