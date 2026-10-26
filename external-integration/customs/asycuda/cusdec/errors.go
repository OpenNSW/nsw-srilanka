package cusdec

import "errors"

// ErrWorkflowNotFoundByEdgeID indicates no task workflow matches the given edgeId.
// This is a permanent condition — the caller should not retry.
var ErrWorkflowNotFoundByEdgeID = errors.New("workflow not found by edgeId")

// ErrDeclarationNotRecorded indicates an integration result arrived for an
// edgeId the dispatch step has not recorded yet. Transient: the step records it
// as soon as SLC Edge accepts the submission, so the caller should retry.
var ErrDeclarationNotRecorded = errors.New("no declaration recorded for edgeId")

// ErrCusdecNotFoundByRef indicates no Customs Declaration matches the given reference.
// This can be a transient condition if an event callback arrives before the integration
// result callback has finished processing — the caller should retry.
var ErrCusdecNotFoundByRef = errors.New("customs declaration not found by reference")

// ErrInvalidCallbackPayload indicates the ASYCUDA callback payload was
// malformed, missing required fields, or otherwise failed validation.
var ErrInvalidCallbackPayload = errors.New("invalid callback payload")

// ErrDuplicateIntegrationResult indicates an integration result has already
// been recorded for this edgeId. It is not a failure: §2 has SLC Edge retry a
// delivery up to four times, so a repeat is expected whenever our 200 was lost
// rather than never sent. The callback is acknowledged and nothing is re-run.
var ErrDuplicateIntegrationResult = errors.New("integration result already processed for edgeId")

// ErrDuplicateEvent indicates a §6.5 event notification has already been
// applied to the declaration it names. Like a repeated integration result this
// is expected under the §2 retry schedule, and is acknowledged without
// modifying the declaration or advancing its workflow.
var ErrDuplicateEvent = errors.New("event notification already applied for cusdecRef")

// ErrCaseNotRecorded indicates the declaration a notification names was never
// linked to the case it was dispatched from, so nothing can be opened or
// resumed on that case for it. Permanent: a retry finds the same row.
var ErrCaseNotRecorded = errors.New("customs declaration has no case recorded")
