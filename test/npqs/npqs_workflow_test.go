package npqs_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	engine "github.com/OpenNSW/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/testsuite"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

type ItemFlowState struct {
	ID     string
	Name   string
	Track  string
	Stage  string
	Status string
}

type WorkflowFlowLogger struct {
	mu          sync.Mutex
	items       map[string]*ItemFlowState
	stepCounter int
}

func newWorkflowFlowLogger(items []any) *WorkflowFlowLogger {
	flowItems := make(map[string]*ItemFlowState, len(items))
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		name, _ := m["commodity_common_name"].(string)
		flowItems[id] = &ItemFlowState{
			ID:     id,
			Name:   name,
			Track:  "Pending Routing",
			Stage:  "Application Submitted",
			Status: "Declared by Trader",
		}
	}
	return &WorkflowFlowLogger{items: flowItems}
}

func (l *WorkflowFlowLogger) LogTransition(stageName, taskTemplateID string, affectedItemIDs []string, actionDetails string, statusUpdates map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.stepCounter++
	fmt.Printf("\n%s\n", strings.Repeat("═", 108))
	fmt.Printf(" [STEP %02d] STAGE: %-28s | TASK: %s\n", l.stepCounter, stageName, taskTemplateID)
	if actionDetails != "" {
		fmt.Printf(" Details: %s\n", actionDetails)
	}
	fmt.Printf(" Active Item(s) in Step: %s\n", strings.Join(affectedItemIDs, ", "))
	fmt.Printf("%s\n", strings.Repeat("─", 108))

	for itemID, newStatus := range statusUpdates {
		if item, ok := l.items[itemID]; ok {
			item.Stage = stageName
			item.Status = newStatus
		}
	}

	fmt.Printf(" %-8s | %-20s | %-26s | %-22s | %s\n", "ITEM ID", "COMMODITY", "ASSIGNED TRACK", "CURRENT STAGE", "STATUS")
	fmt.Printf(" %-8s-+-%-20s-+-%-26s-+-%-22s-+-%s\n", "--------", "--------------------", "--------------------------", "----------------------", "-----------------------------------")
	for _, id := range []string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"} {
		item := l.items[id]
		if item == nil {
			continue
		}
		marker := "  "
		for _, aff := range affectedItemIDs {
			if aff == id {
				marker = "► "
				break
			}
		}
		fmt.Printf("%s%-7s | %-20s | %-26s | %-22s | %s\n", marker, item.ID, item.Name, item.Track, item.Stage, item.Status)
	}
	fmt.Printf("%s\n", strings.Repeat("═", 108))
}

func (l *WorkflowFlowLogger) LogFinalSummary(workflowID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	fmt.Printf("\n%s\n", strings.Repeat("█", 108))
	fmt.Printf(" CONSIGNMENT WORKFLOW COMPLETED: %s\n", workflowID)
	fmt.Printf(" Parallel Tracks Synchronized at PARALLEL_JOIN -> Consignment Clearance -> Certified\n")
	fmt.Printf("%s\n", strings.Repeat("─", 108))
	fmt.Printf(" %-8s | %-20s | %-26s | %-22s | %s\n", "ITEM ID", "COMMODITY", "COMPLETED TRACK", "FINAL STAGE", "FINAL STATUS")
	fmt.Printf(" %-8s-+-%-20s-+-%-26s-+-%-22s-+-%s\n", "--------", "--------------------", "--------------------------", "----------------------", "-----------------------------------")
	for _, id := range []string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"} {
		item := l.items[id]
		if item != nil {
			fmt.Printf("  %-7s | %-20s | %-26s | %-22s | %s\n", item.ID, item.Name, item.Track, item.Stage, item.Status)
		}
	}
	fmt.Printf("%s\n\n", strings.Repeat("█", 108))
}

func loadNPQSWorkflowDefinition(t *testing.T) engine.WorkflowDefinition {
	t.Helper()
	path := "../../../one-trade-artifacts/tnsw/npqs_v2/npqs_workflow.json"
	bytes, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read npqs_workflow.json at %s", path)

	var def engine.WorkflowDefinition
	err = json.Unmarshal(bytes, &def)
	require.NoError(t, err, "failed to unmarshal npqs_workflow.json")
	return def
}

func TestNPQSWorkflow_StructureAndValidation(t *testing.T) {
	def := loadNPQSWorkflowDefinition(t)

	assert.Equal(t, "npqs-v2-export-phytosanitary-reg", def.ID)
	assert.NotEmpty(t, def.Name)

	// 1. Verify batch gateway structure passes ValidateBatchGateways topological checks
	err := engine.ValidateBatchGateways(def)
	require.NoError(t, err, "NPQS workflow must satisfy ValidateBatchGateways invariants")

	// 2. Verify critical structural nodes exist
	nodeMap := make(map[string]engine.Node, len(def.Nodes))
	for _, n := range def.Nodes {
		nodeMap[n.ID] = n
	}

	require.Contains(t, nodeMap, "start")
	require.Contains(t, nodeMap, "n1_apply")
	require.Contains(t, nodeMap, "gw_par_split")
	require.Contains(t, nodeMap, "gw_lab_split")
	require.Contains(t, nodeMap, "gw_lab_join")
	require.Contains(t, nodeMap, "gw_visual_split")
	require.Contains(t, nodeMap, "gw_visual_join")
	require.Contains(t, nodeMap, "gw_treatment_split")
	require.Contains(t, nodeMap, "gw_treatment_join")
	require.Contains(t, nodeMap, "gw_par_join")
	require.Contains(t, nodeMap, "gw_consignment_clearance")
	require.Contains(t, nodeMap, "end_success")
	require.Contains(t, nodeMap, "end_consignment_rejected")

	// 3. Verify gateway types
	assert.Equal(t, engine.GatewayTypeParallelSplit, nodeMap["gw_par_split"].GatewayType)
	assert.Equal(t, engine.GatewayTypeParallelJoin, nodeMap["gw_par_join"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchSplit, nodeMap["gw_lab_split"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchJoin, nodeMap["gw_lab_join"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchSplit, nodeMap["gw_visual_split"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchJoin, nodeMap["gw_visual_join"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchSplit, nodeMap["gw_treatment_split"].GatewayType)
	assert.Equal(t, engine.GatewayTypeBatchJoin, nodeMap["gw_treatment_join"].GatewayType)
	assert.Equal(t, engine.GatewayTypeExclusiveSplit, nodeMap["gw_consignment_clearance"].GatewayType)

	// 4. Verify batch gateway paired IDs and variables
	assert.Equal(t, "commodities", nodeMap["gw_lab_split"].BatchGateway.ItemsVariable)
	assert.Equal(t, "gw_lab_split", nodeMap["gw_lab_join"].BatchJoin.GatewayNodeID)
	assert.Equal(t, "commodities", nodeMap["gw_visual_split"].BatchGateway.ItemsVariable)
	assert.Equal(t, "gw_visual_split", nodeMap["gw_visual_join"].BatchJoin.GatewayNodeID)
	assert.Equal(t, "commodities", nodeMap["gw_treatment_split"].BatchGateway.ItemsVariable)
	assert.Equal(t, "gw_treatment_split", nodeMap["gw_treatment_join"].BatchJoin.GatewayNodeID)
}

func TestNPQSWorkflow_FullExecutionSimulation(t *testing.T) { //nolint:gocyclo // one mock activity handler switching on task_template_id across all three tracks end-to-end; splitting it would scatter the flow this test exists to show in one place
	def := loadNPQSWorkflowDefinition(t)

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	// 4 declared items with different pathways
	declaredItems := []any{
		map[string]any{
			"id":                    "item-1",
			"commodity_common_name": "Cut Rose Flowers",
			"lab_required":          true,
			"visual_required":       false,
			"treatment_required":    false,
		},
		map[string]any{
			"id":                    "item-2",
			"commodity_common_name": "Fresh Cut Foliage",
			"lab_required":          false,
			"visual_required":       true,
			"visual_approach":       "consignment",
			"treatment_required":    false,
		},
		map[string]any{
			"id":                    "item-3",
			"commodity_common_name": "Sawn Timber",
			"lab_required":          false,
			"visual_required":       false,
			"treatment_required":    true,
			"treatment_provider":    "npqs",
			"treatment_supervision": "without_supervision",
		},
		map[string]any{
			"id":                    "item-4",
			"commodity_common_name": "Coir Pith Block",
			"lab_required":          false,
			"visual_required":       false,
			"treatment_required":    false,
		},
		map[string]any{
			"id":                    "item-5",
			"commodity_common_name": "Cinnamon Quills",
			"lab_required":          true,
			"visual_required":       false,
			"treatment_required":    false,
		},
		map[string]any{
			"id":                    "item-6",
			"commodity_common_name": "Dried Cinnamon Bark",
			"lab_required":          false,
			"visual_required":       true,
			"visual_approach":       "sample",
			"treatment_required":    false,
		},
		// item-7 and item-8 join item-3 in the SAME treatment_provider_split
		// batch, each choosing a different provider (item-7: external) or
		// needing supervision within the npqs provider (item-8), to prove a
		// mixed-provider/mixed-supervision batch routes each item down its
		// own path rather than all following whichever single value the old
		// (pre-fix) workflow-level npqs.treatment_provider/treatment_supervision
		// variables happened to hold.
		map[string]any{
			"id":                    "item-7",
			"commodity_common_name": "Bamboo Poles",
			"lab_required":          false,
			"visual_required":       false,
			"treatment_required":    true,
			"treatment_provider":    "external",
			"treatment_supervision": "without_supervision",
		},
		map[string]any{
			"id":                    "item-8",
			"commodity_common_name": "Areca Nut",
			"lab_required":          false,
			"visual_required":       false,
			"treatment_required":    true,
			"treatment_provider":    "npqs",
			"treatment_supervision": "with_supervision",
		},
	}

	flowLogger := newWorkflowFlowLogger(declaredItems)
	executedTasks := make(map[string]int)

	acts := &engine.Activities{
		ExecuteTaskActivityHandler: func(p engine.TaskPayload) (map[string]any, error) {
			executedTasks[p.TaskTemplateID]++
			switch p.TaskTemplateID {
			case "npqs-v2-apply-phyto-cert":
				// Update track assignments
				flowLogger.items["item-1"].Track = "Track 1: Lab Testing"
				flowLogger.items["item-2"].Track = "Track 2: Visual Inspection"
				flowLogger.items["item-3"].Track = "Track 3: Treatment Pipeline"
				flowLogger.items["item-4"].Track = "Fast-Track (Direct Join)"
				flowLogger.items["item-5"].Track = "Track 1: Lab Testing"
				flowLogger.items["item-6"].Track = "Track 2: Visual Inspection (Sample)"
				flowLogger.items["item-7"].Track = "Track 3: Treatment Pipeline (External)"
				flowLogger.items["item-8"].Track = "Track 3: Treatment Pipeline (NPQS, Supervised)"

				flowLogger.LogTransition(
					"1-Apply & Officer Review",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"},
					"Officer approved application & configured per-item inspection/treatment requirements",
					map[string]string{
						"item-1": "Awaiting Sample Collection (Lab Required)",
						"item-2": "Awaiting Consignment Inspection (Visual Required)",
						"item-3": "Awaiting Treatment Plan (NPQS Station Treatment Required)",
						"item-4": "Fast-Tracked: Bypasses to Join (No Intervention Needed)",
						"item-5": "Awaiting Sample Collection (Lab Required)",
						"item-6": "Awaiting Sample Collection (Visual Sample Required)",
						"item-7": "Awaiting Treatment Plan (External Provider Treatment Required)",
						"item-8": "Awaiting Treatment Plan (NPQS Station, Supervision Required)",
					},
				)
				return map[string]any{
					"reviewerform": map[string]any{
						"reference_number": "NPQS-2026-EXP-1092",
						"review_outcome":   "approve",
						"ephyto_required":  true,
						"commodities":      declaredItems,
					},
					"userform": map[string]any{
						"applicant_name": "Lanka Agro Exports Ltd",
					},
				}, nil

			case "npqs-v2-draw-sample":
				flowLogger.LogTransition(
					"3-Lab: Draw Sample",
					p.TaskTemplateID,
					[]string{"item-1", "item-5"},
					"Quarantine officer collected official samples SMP-2026-001 (Cut Rose Flowers) and SMP-2026-002 (Cinnamon Quills)",
					map[string]string{
						"item-1": "Sample SMP-2026-001 Drawn (Queued for Laboratory Diagnostic Testing)",
						"item-5": "Sample SMP-2026-002 Drawn (Queued for Laboratory Diagnostic Testing)",
					},
				)
				return map[string]any{
					"sample_number": "SMP-2026-001",
				}, nil

			case "npqs-v2-lab-testing":
				// Per-item outcomes in the SAME batch submission: item-1 passes while
				// item-5 fails final. This is the scenario the per-item BATCH_SPLIT/JOIN
				// redesign of lab_result_split exists to support — one item can be
				// rejected while the rest of the batch continues, without forcing a
				// resubmission of the whole batch.
				flowLogger.LogTransition(
					"3-Lab: Lab Testing",
					p.TaskTemplateID,
					[]string{"item-1", "item-5"},
					"NPQS Plant Pathology Lab completed tests: item-1 clean, item-5 positive for quarantine pest",
					map[string]string{
						"item-1": "Lab Test PASSED (Diagnostic Result: Clean)",
						"item-5": "Lab Test FAILED FINAL (Diagnostic Result: Quarantine Pest Detected)",
					},
				)
				return map[string]any{
					"sample_test_result": "fail_final",
					"lab_comments":       "item-1 clean; item-5 positive for Bactrocera dorsalis, rejected.",
					"commodities": []any{
						map[string]any{
							"id":                    "item-1",
							"commodity_common_name": "Cut Rose Flowers",
							"sample_test_result":    "pass",
						},
						map[string]any{
							"id":                    "item-5",
							"commodity_common_name": "Cinnamon Quills",
							"sample_test_result":    "fail_final",
							"failure_reason":        "Presence of quarantine pest: Bactrocera dorsalis",
						},
					},
				}, nil

			case "npqs-v2-visual-consignment-flow":
				flowLogger.LogTransition(
					"4-Visual: Consignment Inspection",
					p.TaskTemplateID,
					[]string{"item-2"},
					"Packhouse inspection completed for Fresh Cut Foliage: Free from regulated pests",
					map[string]string{
						"item-2": "Visual Inspection PASSED (Consignment Cleared for Export)",
					},
				)
				return map[string]any{
					"visual_result": "pass",
				}, nil

			case "npqs-v2-visual-sample-collection":
				flowLogger.LogTransition(
					"4-Visual: Collect Sample",
					p.TaskTemplateID,
					[]string{"item-6"},
					"Trader dropped off a representative sample; NPQS officer received and registered it as VSMP-2026-001",
					map[string]string{
						"item-6": "Sample VSMP-2026-001 Received (Queued for Visual Inspection)",
					},
				)
				return map[string]any{
					"sample_number": "VSMP-2026-001",
				}, nil

			case "npqs-v2-visual-sample-inspection":
				flowLogger.LogTransition(
					"4-Visual: Sample Inspection",
					p.TaskTemplateID,
					[]string{"item-6"},
					"Quarantine officer visually inspected sample VSMP-2026-001: free from regulated pests",
					map[string]string{
						"item-6": "Visual Sample Inspection PASSED (Cleared for Export)",
					},
				)
				return map[string]any{
					"visual_result": "pass",
				}, nil

			case "npqs-v2-treatment-request":
				// item-3, item-7, item-8 all satisfy treatment_required == true,
				// so gw_treatment_split groups them into ONE n3_treatment_request
				// batch together (partitioned by shared edge condition value, not
				// per item) - even though they'll fan back out to different
				// providers (and, within the npqs provider, different supervision
				// needs) immediately after via the nested treatment_provider_split
				// / n3_3_1_supervision_split batch gateways.
				flowLogger.LogTransition(
					"5-Treatment: Request & Review",
					p.TaskTemplateID,
					[]string{"item-3", "item-7", "item-8"},
					"Treatment plan approved: item-3/item-8 at NPQS Station, item-7 via external provider",
					map[string]string{
						"item-3": "Treatment Request APPROVED (Assigned to NPQS Station)",
						"item-7": "Treatment Request APPROVED (Assigned to External Provider)",
						"item-8": "Treatment Request APPROVED (Assigned to NPQS Station, Supervision Required)",
					},
				)
				return map[string]any{
					"reviewerform": map[string]any{
						"treatment_request_outcome": "approve",
					},
				}, nil

			case "npqs-v2-pay-for-treatment":
				// npqs provider partition only (item-3, item-8) - item-7 (external)
				// never reaches this node.
				flowLogger.LogTransition(
					"5-Treatment: Station Payment",
					p.TaskTemplateID,
					[]string{"item-3", "item-8"},
					"Trader settled NPQS treatment facility fees via online portal",
					map[string]string{
						"item-3": "Treatment Fee PAID (Authorized for Fumigation)",
						"item-8": "Treatment Fee PAID (Authorized for Fumigation)",
					},
				)
				return map[string]any{
					"payment_status": "success",
				}, nil

			case "npqs-v2-issue-treatment-cert":
				flowLogger.LogTransition(
					"5-Treatment: Certificate Issuance",
					p.TaskTemplateID,
					[]string{"item-3", "item-8"},
					"NPQS treatment officer issued official Treatment Certificate TC-NPQS-901",
					map[string]string{
						"item-3": "Treatment Certificate TC-NPQS-901 ISSUED",
						"item-8": "Treatment Certificate TC-NPQS-901 ISSUED",
					},
				)
				return map[string]any{
					"treatment_certificate_id": "TC-NPQS-901",
				}, nil

			case "npqs-v2-treatment-supervisor-report":
				// Only item-8 (with_supervision) reaches this node - item-3
				// (without_supervision) bypasses it via
				// n3_3_1_supervision_split's false edge straight to the
				// supervision join. Proves the nested supervision BATCH_SPLIT
				// correctly separates item-3 from item-8 even though both are
				// in the same npqs-provider partition.
				flowLogger.LogTransition(
					"5-Treatment: Supervision Report",
					p.TaskTemplateID,
					[]string{"item-8"},
					"NPQS supervising officer completed on-site observation report",
					map[string]string{
						"item-8": "Supervision Report Issued (Supervised Treatment Complete)",
					},
				)
				return map[string]any{
					"supervisor_notes": "Treatment executed to specification under direct NPQS supervision.",
				}, nil

			case "npqs-v2-upload-treatment-certs":
				// Reached twice: once for the npqs-provider partition
				// (item-3+item-8, after the supervision sub-batch rejoins) and
				// once for the external-provider partition (item-7 alone).
				flowLogger.LogTransition(
					"5-Treatment: Cert Upload",
					p.TaskTemplateID,
					[]string{"item-3", "item-7", "item-8"},
					"Trader attached signed treatment certificate for quarantine verification",
					map[string]string{
						"item-3": "Treatment Certificate Document Uploaded",
						"item-7": "Treatment Certificate Document Uploaded",
						"item-8": "Treatment Certificate Document Uploaded",
					},
				)
				return map[string]any{
					"traderinput": map[string]any{
						"certificate_url": "https://nsw.gov.lk/storage/cert-901.pdf",
					},
				}, nil

			case "npqs-v2-review-treatment-certs":
				flowLogger.LogTransition(
					"5-Treatment: Cert Verification",
					p.TaskTemplateID,
					[]string{"item-3", "item-7", "item-8"},
					"NPQS officer verified treatment parameters: Treatment verified & cleared",
					map[string]string{
						"item-3": "Treatment VERIFIED & PASSED (Treatment Pipeline Complete)",
						"item-7": "Treatment VERIFIED & PASSED (Treatment Pipeline Complete)",
						"item-8": "Treatment VERIFIED & PASSED (Treatment Pipeline Complete)",
					},
				)
				return map[string]any{
					"treatment_review_result": "pass",
				}, nil

			case "npqs-v2-upload-docs":
				flowLogger.LogTransition(
					"6-Docs: Upload Trade Documents",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"},
					"[PARALLEL_JOIN Reached] All tracks synchronized; Trader uploaded invoice & packing list",
					map[string]string{
						"item-1": "All Tracks Synchronized: Trade Documents Uploaded",
						"item-2": "All Tracks Synchronized: Trade Documents Uploaded",
						"item-3": "All Tracks Synchronized: Trade Documents Uploaded",
						"item-4": "All Tracks Synchronized: Trade Documents Uploaded",
						"item-5": "All Tracks Synchronized: Trade Documents Uploaded (Rejected By Lab)",
						"item-6": "All Tracks Synchronized: Trade Documents Uploaded",
					},
				)
				return map[string]any{
					"traderinput": map[string]any{
						"invoice_url": "https://nsw.gov.lk/storage/inv-001.pdf",
					},
				}, nil

			case "npqs-v2-review-docs":
				flowLogger.LogTransition(
					"6-Docs: Review Trade Documents",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"},
					"NPQS documentation officer verified and approved commercial documents",
					map[string]string{
						"item-1": "Trade Documents APPROVED (Consignment Cleared for Payment)",
						"item-2": "Trade Documents APPROVED (Consignment Cleared for Payment)",
						"item-3": "Trade Documents APPROVED (Consignment Cleared for Payment)",
						"item-4": "Trade Documents APPROVED (Consignment Cleared for Payment)",
						"item-5": "Trade Documents APPROVED (Consignment Cleared for Payment; Rejected By Lab)",
						"item-6": "Trade Documents APPROVED (Consignment Cleared for Payment)",
					},
				)
				return map[string]any{
					"docs_review_outcome": "approve",
				}, nil

			case "npqs-v2-pay-certificate-fee":
				flowLogger.LogTransition(
					"7-Payment: Phytosanitary Certificate Fee",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"},
					"Trader completed statutory phytosanitary certificate fee payment",
					map[string]string{
						"item-1": "Certificate Fee PAID (Authorized for Final Issuance)",
						"item-2": "Certificate Fee PAID (Authorized for Final Issuance)",
						"item-3": "Certificate Fee PAID (Authorized for Final Issuance)",
						"item-4": "Certificate Fee PAID (Authorized for Final Issuance)",
						"item-5": "Certificate Fee PAID (Rejected By Lab; Excluded From Certificate)",
						"item-6": "Certificate Fee PAID (Authorized for Final Issuance)",
					},
				)
				return map[string]any{
					"payment_status": "success",
				}, nil

			case "npqs-v2-issue-certificate":
				// The officer's item picker is prefilled from `commodities` with id +
				// commodity name for every declared item (that part never races: it's
				// the pristine list n1_apply produced, untouched by any track). Per-item
				// track OUTCOME context (lab/visual/treatment result) is deliberately
				// NOT shown here: gw_lab_split, gw_visual_split, and gw_treatment_split
				// all run concurrently under gw_par_split and, as currently built, share
				// a single "commodities" items_variable — each one's completion
				// overwrites that variable wholesale, so whichever track finishes last
				// silently clobbers the others' per-item merges. That's a real gap in
				// the shared core workflow engine (BATCH_JOIN has no merge-by-id
				// semantics for concurrent writers), not something fixed here — so the
				// picker only asserts on what's actually reliable: item identity.
				certItemsIn, _ := p.Inputs["certificate_items"].([]any)
				gotIDs := make(map[string]bool, len(certItemsIn))
				for _, raw := range certItemsIn {
					item, _ := raw.(map[string]any)
					if id, _ := item["id"].(string); id != "" {
						gotIDs[id] = true
					}
				}
				for _, wantID := range []string{"item-1", "item-2", "item-3", "item-4", "item-5", "item-6", "item-7", "item-8"} {
					assert.True(t, gotIDs[wantID], "certificate item picker must be prefilled with %s, got %+v", wantID, certItemsIn)
				}

				flowLogger.LogTransition(
					"8-Issuance: Phytosanitary Certificate",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-6", "item-7", "item-8"},
					"Senior NPQS Quarantine Officer issued Phytosanitary Certificate PC-NPQS-2026-8092 (item-5 excluded: rejected by lab)",
					map[string]string{
						"item-1": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-2": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-3": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-4": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-5": "EXCLUDED From Certificate (Rejected By Lab Test)",
						"item-6": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-7": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
						"item-8": "Phytosanitary Certificate PC-NPQS-2026-8092 ISSUED",
					},
				)
				// Officer deselects item-5 (and only item-5) on the picker.
				return map[string]any{
					"certificate_id": "PC-NPQS-2026-8092",
					"certificate_items": []any{
						map[string]any{"id": "item-1", "include_in_certificate": true},
						map[string]any{"id": "item-2", "include_in_certificate": true},
						map[string]any{"id": "item-3", "include_in_certificate": true},
						map[string]any{"id": "item-4", "include_in_certificate": true},
						map[string]any{"id": "item-5", "include_in_certificate": false},
						map[string]any{"id": "item-6", "include_in_certificate": true},
						map[string]any{"id": "item-7", "include_in_certificate": true},
						map[string]any{"id": "item-8", "include_in_certificate": true},
					},
				}, nil

			case "npqs-v2-ephyto-upload":
				// The officer's exclusion of item-5 at certificate issuance must have
				// survived through to the ePhyto submission task's own inputs — this
				// is what build.go's excludedItemIDs() reads to keep item-5 off the
				// transmitted XML.
				certItemsOut, _ := p.Inputs["certificate_items"].([]any)
				var sawItem5Excluded bool
				for _, raw := range certItemsOut {
					item, _ := raw.(map[string]any)
					if item["id"] == "item-5" && item["include_in_certificate"] == false {
						sawItem5Excluded = true
					}
				}
				assert.True(t, sawItem5Excluded, "item-5's certificate exclusion must reach the ePhyto submission task, got %+v", certItemsOut)

				flowLogger.LogTransition(
					"9-ePhyto: IPPC Hub Transmission",
					p.TaskTemplateID,
					[]string{"item-1", "item-2", "item-3", "item-4", "item-6", "item-7", "item-8"},
					"Electronic Phytosanitary Certificate XML successfully transmitted to IPPC ePhyto Hub (item-5 excluded)",
					map[string]string{
						"item-1": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-2": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-3": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-4": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-6": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-7": "ePhyto Hub Transmission Confirmed (Completed)",
						"item-8": "ePhyto Hub Transmission Confirmed (Completed)",
					},
				)
				return map[string]any{
					"ephyto_status": "success",
				}, nil

			default:
				return map[string]any{}, nil
			}
		},
		WorkflowCompletedActivityHandler: func(workflowID string, finalContext map[string]any) error {
			if !strings.Contains(workflowID, "--") {
				flowLogger.LogFinalSummary(workflowID)
			}
			return nil
		},
	}

	env.RegisterActivityWithOptions(acts.ExecuteTaskActivity, activity.RegisterOptions{Name: "ExecuteTaskActivity"})
	env.RegisterActivityWithOptions(acts.WorkflowCompletedActivity, activity.RegisterOptions{Name: "WorkflowCompletedActivity"})
	env.RegisterWorkflowWithOptions(engine.GraphInterpreterWorkflow, temporalworkflow.RegisterOptions{Name: "GraphInterpreterWorkflow"})
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "npqs-unit-test-1"})

	env.ExecuteWorkflow(engine.GraphInterpreterWorkflow, def, map[string]any{})

	require.True(t, env.IsWorkflowCompleted(), "workflow must complete")
	require.NoError(t, env.GetWorkflowError(), "workflow execution must not error")

	// Verify all expected tasks across the 3 parallel tracks ran
	assert.Equal(t, 1, executedTasks["npqs-v2-apply-phyto-cert"], "n1_apply must execute once")
	assert.Equal(t, 1, executedTasks["npqs-v2-draw-sample"], "Track 1 (Lab) draw sample must execute once for the item-1/item-5 batch")
	// Exactly one lab-testing call, and no second draw-sample/retest call, proves item-5's
	// fail_final and item-1's pass were each resolved directly from the same submission —
	// the per-item BATCH_SPLIT (lab_result_split) does not force the whole batch through
	// the resubmit loop just because one item in it was rejected.
	assert.Equal(t, 1, executedTasks["npqs-v2-lab-testing"], "Track 1 (Lab) testing must execute once for the item-1/item-5 batch, with no resubmit retest triggered")
	assert.Equal(t, 1, executedTasks["npqs-v2-visual-consignment-flow"], "Track 2 (Visual) must execute for item-2")
	assert.Equal(t, 1, executedTasks["npqs-v2-visual-sample-collection"], "Track 2 (Visual/Sample) sample collection must execute for item-6")
	assert.Equal(t, 1, executedTasks["npqs-v2-visual-sample-inspection"], "Track 2 (Visual/Sample) inspection must execute for item-6, after sample collection")
	assert.Equal(t, 1, executedTasks["npqs-v2-treatment-request"], "Track 3 (Treatment) must execute once for the item-3/item-7/item-8 batch")
	// treatment_provider_split now partitions per item (item.treatment_provider),
	// not on a single workflow-level npqs.treatment_provider value set by the
	// officer's own single decision - this is the regression guard for that fix.
	// item-8 (npqs, with_supervision) reaching the supervisor-report node while
	// item-3 (npqs, without_supervision) does NOT proves the nested
	// n3_3_1_supervision_split ALSO correctly partitions per item within the
	// npqs provider branch.
	assert.Equal(t, 1, executedTasks["npqs-v2-pay-for-treatment"], "Track 3 payment must execute once for the item-3/item-8 (npqs-provider) batch")
	assert.Equal(t, 1, executedTasks["npqs-v2-issue-treatment-cert"], "Track 3 cert issue must execute once for the item-3/item-8 (npqs-provider) batch")
	assert.Equal(t, 1, executedTasks["npqs-v2-treatment-supervisor-report"], "supervision report must execute once, for item-8 only (with_supervision)")
	// Reached from BOTH provider partitions (npqs: item-3+item-8 after the
	// supervision sub-batch rejoins; external: item-7 alone) - each partition
	// runs its own separate call, so this fires twice, not once. That's the
	// real per-partition execution count; the mock's own LogTransition calls
	// for these two cases print the same static item-3/item-7/item-8 label on
	// both, since the mock has no way to tell which partition it's being
	// called for.
	assert.Equal(t, 2, executedTasks["npqs-v2-upload-treatment-certs"], "cert upload must execute once per provider partition (npqs, external)")
	assert.Equal(t, 2, executedTasks["npqs-v2-review-treatment-certs"], "cert review must execute once per provider partition (npqs, external)")
	assert.Equal(t, 1, executedTasks["npqs-v2-upload-docs"], "Consignment doc upload must execute")
	assert.Equal(t, 1, executedTasks["npqs-v2-review-docs"], "Consignment doc review must execute")
	assert.Equal(t, 1, executedTasks["npqs-v2-pay-certificate-fee"], "Consignment certificate fee payment must execute")
	assert.Equal(t, 1, executedTasks["npqs-v2-issue-certificate"], "Phytosanitary certificate issuance must execute")
	assert.Equal(t, 1, executedTasks["npqs-v2-ephyto-upload"], "ePhyto IPPC Hub upload must execute")
}

func TestNPQSWorkflow_AllItemsFailed_ConsignmentRejected(t *testing.T) {
	def := loadNPQSWorkflowDefinition(t)

	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()

	declaredItems := []any{
		map[string]any{
			"id":                    "item-1",
			"commodity_common_name": "Cut Rose Flowers",
			"lab_required":          false,
			"visual_required":       true,
			"visual_approach":       "consignment",
			"treatment_required":    false,
		},
	}

	executedTasks := make(map[string]int)

	acts := &engine.Activities{
		ExecuteTaskActivityHandler: func(p engine.TaskPayload) (map[string]any, error) {
			executedTasks[p.TaskTemplateID]++
			switch p.TaskTemplateID {
			case "npqs-v2-apply-phyto-cert":
				return map[string]any{
					"reviewerform": map[string]any{
						"reference_number": "NPQS-2026-EXP-9999",
						"review_outcome":   "approve",
						"ephyto_required":  false,
						"commodities":      declaredItems,
					},
					"userform": map[string]any{
						"applicant_name": "Agro Fails Inc",
					},
				}, nil

			case "npqs-v2-visual-consignment-flow":
				// Visual inspection fails for all items
				return map[string]any{
					"visual_result": "fail",
				}, nil

			default:
				return map[string]any{}, nil
			}
		},
		WorkflowCompletedActivityHandler: func(workflowID string, finalContext map[string]any) error {
			return nil
		},
	}

	env.RegisterActivityWithOptions(acts.ExecuteTaskActivity, activity.RegisterOptions{Name: "ExecuteTaskActivity"})
	env.RegisterActivityWithOptions(acts.WorkflowCompletedActivity, activity.RegisterOptions{Name: "WorkflowCompletedActivity"})
	env.RegisterWorkflowWithOptions(engine.GraphInterpreterWorkflow, temporalworkflow.RegisterOptions{Name: "GraphInterpreterWorkflow"})
	env.SetStartWorkflowOptions(client.StartWorkflowOptions{ID: "npqs-unit-test-reject"})

	// Pre-seed all_items_failed = true to simulate clearance rejection evaluation
	env.ExecuteWorkflow(engine.GraphInterpreterWorkflow, def, map[string]any{
		"npqs": map[string]any{
			"all_items_failed": true,
		},
	})

	require.True(t, env.IsWorkflowCompleted(), "workflow must complete")
	require.NoError(t, env.GetWorkflowError(), "workflow execution must not error")

	assert.Equal(t, 1, executedTasks["npqs-v2-apply-phyto-cert"], "n1_apply must execute")
	assert.Equal(t, 1, executedTasks["npqs-v2-visual-consignment-flow"], "Visual inspection must execute and fail")
	assert.Equal(t, 0, executedTasks["npqs-v2-upload-docs"], "Consignment doc upload must NOT execute on rejection")
	assert.Equal(t, 0, executedTasks["npqs-v2-pay-certificate-fee"], "Payment must NOT execute on rejection")
	assert.Equal(t, 0, executedTasks["npqs-v2-issue-certificate"], "Certificate issuance must NOT execute on rejection")
}
