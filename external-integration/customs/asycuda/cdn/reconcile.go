package cdn

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/parkedtask"
)

// RetrieveDeclaration calls SLC Edge's declaration retrieve (§6.7) and returns
// its answer.
type RetrieveDeclaration func(ctx context.Context, req document.DeclarationRetrieve) (map[string]any, error)

// Reconciler brings a case's dispatch note branches in line with the containers
// an amended declaration (§6.5.4) has.
//
// The consignment opens one branch per container once the declaration is
// warranted. When Customs amends the declaration to fewer containers, the
// branches for the containers it dropped are withdrawn: a branch still on the
// trader's form, or sent and waiting to be integrated, is completed as
// cancelled. A branch whose note ASYCUDA registered is left alone; only Customs
// can cancel a registered note, and its §7.7 notification ends that branch.
// Customs has confirmed an amendment never adds a container.
//
// Before it withdraws anything, it retrieves the declaration as ASYCUDA World
// holds it, whose cdnRefs are the notes registered against it, and goes on only
// if NSW holds as many registered notes. Otherwise a callback is still on its
// way, and a branch it is about to integrate could be withdrawn; nothing is
// withdrawn, and the next amendment reconciles again.
type Reconciler struct {
	ledger      Ledger
	repo        DispatchNoteRepository
	taskManager TaskCompleter
	retrieve    RetrieveDeclaration
}

// NewReconciler returns a Reconciler over the case's dispatch notes and tasks,
// completing steps through taskManager and retrieving the declaration through
// retrieve.
func NewReconciler(repo DispatchNoteRepository, tasks parkedtask.Tasks, taskManager TaskCompleter, retrieve RetrieveDeclaration) *Reconciler {
	return &Reconciler{ledger: NewLedger(repo, tasks), repo: repo, taskManager: taskManager, retrieve: retrieve}
}

// ReconcileContainers withdraws the branches of caseID beyond the containers the
// amended declaration has. An error leaves what is withdrawn so far; the
// amendment's redelivery finds those branches cancelled and goes on from there.
func (r *Reconciler) ReconcileContainers(ctx context.Context, caseID string, ref DocumentReference, amended map[string]any) error {
	containers, ok := containerCount(amended)
	if !ok {
		slog.WarnContext(ctx, "cdn reconcile: the amended declaration carries no usable container count; dispatch notes left as they are",
			"cusdec_ref", ref, "case_id", caseID)
		return nil
	}

	branches, err := r.ledger.Branches(ctx, caseID)
	if err != nil {
		return err
	}
	live := liveBranches(branches)
	logLedger(ctx, "cdn reconcile: dispatch notes before the amendment", caseID, ref, branches, containers)
	// No branches means the declaration has not reached the split, which reads
	// the container count when it does.
	if len(live) <= containers {
		return nil
	}

	if r.retrieve == nil {
		return errors.New("cdn reconcile: no declaration retrieve to check the registered dispatch notes against")
	}
	declarant, company := amendedTraderCodes(amended)
	resp, err := r.retrieve(ctx, document.DeclarationRetrieve{CusdecRef: ref, DeclarantCode: declarant, CompanyCode: company})
	if err != nil {
		return fmt.Errorf("cdn reconcile: retrieve declaration %s: %w", ref, err)
	}
	if decl := document.RetrievedDeclaration(resp); decl != nil {
		if current, ok := containerCount(decl); ok {
			containers = current
		}
	}
	if len(live) <= containers {
		return nil
	}

	notes, err := r.repo.ListByCase(ctx, caseID)
	if err != nil {
		return fmt.Errorf("cdn reconcile: load the dispatch notes of case %s: %w", caseID, err)
	}
	inNSW, inASYCUDA := registeredInNSW(notes), registeredInASYCUDA(document.RetrievedCDNRefs(resp))
	if inNSW != inASYCUDA {
		slog.WarnContext(ctx, "cdn reconcile: NSW and ASYCUDA hold different counts of registered dispatch notes; nothing withdrawn",
			"cusdec_ref", ref, "case_id", caseID, "registered_in_nsw", inNSW, "registered_in_asycuda", inASYCUDA)
		return nil
	}

	excess := len(live) - containers
	candidates := withdrawable(live)
	if len(candidates) < excess {
		slog.WarnContext(ctx, "cdn reconcile: more dispatch notes are registered than the amended declaration has containers; Customs has to cancel the rest",
			"cusdec_ref", ref, "case_id", caseID, "containers", containers, "registered", inNSW)
		excess = len(candidates)
	}
	reason := fmt.Sprintf("Sri Lanka Customs amended declaration %s to %d %s, so this container's dispatch note is no longer needed.",
		ref, containers, plural(containers, "container", "containers"))
	for _, b := range candidates[:excess] {
		if err := r.withdraw(ctx, b, reason); err != nil {
			return err
		}
		slog.InfoContext(ctx, "cdn reconcile: withdrew dispatch note branch",
			"cusdec_ref", ref, "case_id", caseID, "workflow_id", b.WorkflowID, "stage", b.Stage)
	}
	return nil
}

// withdraw completes the step a branch is parked on as cancelled. A branch
// still on the trader's form is cancelled with the form's cancel command; one
// waiting to be integrated is released as not integrated, and its note is
// marked withdrawn so a result that still arrives for it is not taken as the
// branch's.
func (r *Reconciler) withdraw(ctx context.Context, b Branch, reason string) error {
	payload := map[string]any{
		"cancelled":           true,
		"cancellation_reason": reason,
	}
	switch b.Stage {
	case StageNotSubmitted:
		payload["__command"] = "cancel"
	case StageSubmitted:
		payload["__command"] = "submit"
		payload["integrated"] = false
	default:
		return fmt.Errorf("cdn reconcile: a branch at %s cannot be withdrawn", b.Stage)
	}
	if err := r.taskManager.CompleteTaskStep(ctx, b.parked.TaskID, b.parked.StepID, payload); err != nil {
		return fmt.Errorf("cdn reconcile: withdraw branch %s: %w", b.WorkflowID, err)
	}
	if b.Stage == StageSubmitted && b.Note != nil && b.Note.Status == DispatchNoteStatusSubmitted {
		b.Note.Status = DispatchNoteStatusWithdrawn
		if err := r.repo.Update(ctx, b.Note); err != nil {
			return fmt.Errorf("cdn reconcile: record dispatch note %s as WITHDRAWN: %w", b.Note.EdgeID, err)
		}
	}
	return nil
}

// liveBranches are the branches that still stand for a container.
func liveBranches(branches []Branch) []Branch {
	live := make([]Branch, 0, len(branches))
	for _, b := range branches {
		if b.live() {
			live = append(live, b)
		}
	}
	return live
}

// withdrawable are the live branches NSW may withdraw, those still on the
// trader's form before those waiting to be integrated, so a note ASYCUDA may
// be about to register is the last to go.
func withdrawable(live []Branch) []Branch {
	var out []Branch
	for _, stage := range []Stage{StageNotSubmitted, StageSubmitted} {
		for _, b := range live {
			if b.Stage == stage {
				out = append(out, b)
			}
		}
	}
	return out
}

// registeredInNSW counts the case's notes ASYCUDA registered and has not
// cancelled. A withdrawn note counts once a result gave it a reference: ASYCUDA
// registered it all the same.
func registeredInNSW(notes []DispatchNote) int {
	n := 0
	for _, note := range notes {
		ref := DocumentReference{Office: note.CDNOffice, Year: note.CDNYear, Serial: note.CDNSerial, Number: note.CDNNumber}
		if ref.IsValid() && note.Status != DispatchNoteStatusCancelled {
			n++
		}
	}
	return n
}

// registeredInASYCUDA counts the notes a retrieve lists against the
// declaration (cdnRefs), leaving out those it reports cancelled.
func registeredInASYCUDA(cdnRefs []map[string]any) int {
	n := 0
	for _, ref := range cdnRefs {
		if !strings.Contains(strings.ToLower(document.Str(ref, "status")), "cancel") {
			n++
		}
	}
	return n
}

// containerCount reads generalSegment.numberOfContainers from a declaration,
// and whether it is usable.
func containerCount(decl map[string]any) (int, bool) {
	raw, ok := document.Nested(decl, "generalSegment")["numberOfContainers"]
	if !ok {
		return 0, false
	}
	count, err := document.WholeNumber(raw)
	if err != nil || count < 1 {
		return 0, false
	}
	return count, true
}

// amendedTraderCodes are the declarant and the exporter's company code an
// amended declaration names, which the retrieve has to match (§6.7).
func amendedTraderCodes(decl map[string]any) (declarant, company string) {
	base := document.Nested(decl, "baseGeneralSegment")
	return document.Str(base, "declarantCode"), document.Str(document.Nested(base, "exporter"), "id")
}

func logLedger(ctx context.Context, msg, caseID string, ref DocumentReference, branches []Branch, containers int) {
	tally := Tally(branches)
	attrs := []any{"cusdec_ref", ref, "case_id", caseID, "containers", containers, "branches", len(branches)}
	for _, stage := range []Stage{StageNotSubmitted, StageSubmitting, StageSubmitted, StageIntegrated, StageAcknowledged, StageCancelled} {
		attrs = append(attrs, strings.ToLower(string(stage)), tally[stage])
	}
	slog.InfoContext(ctx, msg, attrs...)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
