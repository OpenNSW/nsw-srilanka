package npqs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type agencyTaskConfig struct {
	dir          string
	reviewFormID string
	outcomeField string
}

type agencyFormDoc struct {
	path   string
	schema map[string]any
}

type wfNode struct {
	ID             string            `json:"id"`
	TaskTemplateID string            `json:"task_template_id"`
	OutputMapping  map[string]string `json:"output_mapping"`
}

// TestNPQSAgencySubmitSchemaMatchesWorkflowWiring statically cross-checks every
// EXTERNAL_REVIEW/USER_INPUT task in the NPQS spec against its agency-side counterpart,
// entirely offline (no services, no Temporal) - just the artifact repo on disk. It exists
// because this exact class of drift has bitten this workflow twice already: an
// output_mapping expecting a field the officer's actual submission form doesn't have
// causes a SILENT PARK (the node just never resumes, no error surfaced) rather than a
// loud failure, and a field renamed on one side (tnsw/) without updating its agency
// mirror silently reintroduces whatever bug the rename was meant to fix.
//
// For each tnsw/npqs/*/*.json task-template file with an EXTERNAL_REVIEW or
// USER_INPUT task_type:
//  1. Its plugin_properties.task_code is used to find the matching agency
//     npqs/*/*.taskconfig.json (by taskCode).
//  2. That taskconfig's forms.review names a jsonform ID - resolved against every
//     agency npqs/*/*.json file with a matching top-level "id", to find the actual
//     form the officer/trader submits through in production.
//  3. Every REQUIRED (non-"?") key in the corresponding tnsw workflow.json TASK node's
//     output_mapping must exist as a top-level schema property on that agency form -
//     otherwise the officer's real submission can never satisfy the mapping, and the
//     node parks forever the first time anyone actually uses it.
//
// This only checks direct (non-dotted) output_mapping keys - a dotted source key (e.g.
// "traderinput.treatment_certificate_url") reads from a different data source than the
// officer's own form and isn't checked here.
func TestNPQSAgencySubmitSchemaMatchesWorkflowWiring(t *testing.T) {
	root := "../../../one-trade-artifacts"
	tnswDir := filepath.Join(root, "tnsw", "npqs")
	agencyDir := filepath.Join(root, "npqs")

	agencyByTaskCode := loadAgencyTaskConfigs(t, agencyDir)
	agencyFormsByID := loadAgencyForms(t, agencyDir)

	subDirs, err := filepath.Glob(filepath.Join(tnswDir, "*"))
	require.NoError(t, err)
	sort.Strings(subDirs)

	checked := 0
	for _, subDir := range subDirs {
		info, err := os.Stat(subDir)
		if err != nil || !info.IsDir() {
			continue
		}
		workflowPath := filepath.Join(subDir, "workflow.json")
		if _, err := os.Stat(workflowPath); err != nil {
			continue
		}

		var wf struct {
			Nodes []wfNode `json:"nodes"`
		}
		require.NoError(t, readJSON(t, workflowPath, &wf))

		pluginFiles, err := filepath.Glob(filepath.Join(subDir, "*.json"))
		require.NoError(t, err)
		for _, pluginPath := range pluginFiles {
			if checkAgencyPluginFile(t, root, pluginPath, workflowPath, wf.Nodes, agencyByTaskCode, agencyFormsByID) {
				checked++
			}
		}
	}

	require.Greater(t, checked, 0, "expected to actually check at least one task - the matching logic itself may be broken")
	t.Logf("cross-checked %d EXTERNAL_REVIEW/USER_INPUT tasks against their agency forms", checked)
}

// loadAgencyTaskConfigs indexes every agency npqs/*/*.taskconfig.json by its taskCode.
func loadAgencyTaskConfigs(t *testing.T, agencyDir string) map[string]agencyTaskConfig {
	t.Helper()
	tcFiles, err := filepath.Glob(filepath.Join(agencyDir, "*", "*.taskconfig.json"))
	require.NoError(t, err)
	require.NotEmpty(t, tcFiles, "expected at least one agency taskconfig.json under %s", agencyDir)

	byTaskCode := make(map[string]agencyTaskConfig, len(tcFiles))
	for _, tcPath := range tcFiles {
		var doc struct {
			TaskCode string `json:"taskCode"`
			Forms    struct {
				Review string `json:"review"`
			} `json:"forms"`
			Behavior struct {
				OutcomeField string `json:"outcomeField"`
			} `json:"behavior"`
		}
		require.NoError(t, readJSON(t, tcPath, &doc))
		require.NotEmpty(t, doc.TaskCode, "%s: taskCode is required", tcPath)
		byTaskCode[doc.TaskCode] = agencyTaskConfig{
			dir:          filepath.Dir(tcPath),
			reviewFormID: doc.Forms.Review,
			outcomeField: doc.Behavior.OutcomeField,
		}
	}
	return byTaskCode
}

// loadAgencyForms indexes every agency jsonform file by its own top-level "id".
func loadAgencyForms(t *testing.T, agencyDir string) map[string]agencyFormDoc {
	t.Helper()
	formFiles, err := filepath.Glob(filepath.Join(agencyDir, "*", "*.json"))
	require.NoError(t, err)

	byID := make(map[string]agencyFormDoc, len(formFiles))
	for _, p := range formFiles {
		var doc struct {
			ID     string         `json:"id"`
			Schema map[string]any `json:"schema"`
		}
		if json.Unmarshal(readFile(t, p), &doc) != nil || doc.ID == "" || doc.Schema == nil {
			continue
		}
		byID[doc.ID] = agencyFormDoc{path: p, schema: doc.Schema}
	}
	return byID
}

// checkAgencyPluginFile checks one tnsw task-template file (if it's an EXTERNAL_REVIEW or
// USER_INPUT plugin definition) against its agency counterpart. Returns true if the file
// was actually a task plugin that got checked against its workflow.json node.
func checkAgencyPluginFile(
	t *testing.T,
	root, pluginPath, workflowPath string,
	nodes []wfNode,
	agencyByTaskCode map[string]agencyTaskConfig,
	agencyFormsByID map[string]agencyFormDoc,
) bool {
	t.Helper()

	var plugin struct {
		ID               string `json:"id"`
		TaskType         string `json:"task_type"`
		PluginProperties struct {
			TaskCode string `json:"task_code"`
		} `json:"plugin_properties"`
	}
	if json.Unmarshal(readFile(t, pluginPath), &plugin) != nil {
		return false
	}
	if plugin.TaskType != "EXTERNAL_REVIEW" && plugin.TaskType != "USER_INPUT" {
		return false
	}
	taskCode := plugin.PluginProperties.TaskCode
	if taskCode == "" || plugin.ID == "" {
		return false
	}

	agency, ok := agencyByTaskCode[taskCode]
	if !ok {
		t.Errorf("%s: task_code %q has no matching agency taskconfig", relPath(root, pluginPath), taskCode)
		return false
	}
	form, ok := agencyFormsByID[agency.reviewFormID]
	if !ok {
		t.Errorf("%s: agency taskconfig's forms.review=%q does not match any agency jsonform's own \"id\"", relPath(root, pluginPath), agency.reviewFormID)
		return false
	}
	props, _ := form.schema["properties"].(map[string]any)

	var node *wfNode
	for i := range nodes {
		if nodes[i].TaskTemplateID == plugin.ID {
			node = &nodes[i]
			break
		}
	}
	if node == nil {
		t.Logf("NOTE: %s (task_template_id=%s) is not referenced by any TASK node in %s - likely an orphaned/unused artifact file, not a schema bug",
			relPath(root, pluginPath), plugin.ID, relPath(root, workflowPath))
		return false
	}

	checkOutputMappingAgainstForm(t, root, workflowPath, *node, props, form, agency)
	return true
}

// checkOutputMappingAgainstForm asserts every required (non-"?") output_mapping key exists
// as a schema property on the agency form the officer actually submits through, and that
// the taskconfig's own outcomeField (if any) is likewise a real property on that form.
func checkOutputMappingAgainstForm(t *testing.T, root, workflowPath string, node wfNode, props map[string]any, form agencyFormDoc, agency agencyTaskConfig) {
	t.Helper()

	for rawKey := range node.OutputMapping {
		optional := strings.HasSuffix(rawKey, "?")
		key := strings.TrimSuffix(rawKey, "?")
		if strings.Contains(key, ".") {
			continue // nested source key, not sourced from this form directly
		}
		if _, exists := props[key]; !exists {
			msg := "REQUIRED - the node will silently park the first time an officer submits, since the mapping can never be satisfied"
			if optional {
				msg = "optional - fine if intentional, but confirm this field was meant to be dropped from the agency form"
			}
			t.Errorf("%s node=%s: output_mapping expects field %q, but agency form %s (id=%s) has no such property [%s]",
				relPath(root, workflowPath), node.ID, key, relPath(root, form.path), agency.reviewFormID, msg)
		}
	}

	if agency.outcomeField != "" {
		if _, exists := props[agency.outcomeField]; !exists {
			t.Errorf("%s: taskconfig's own outcomeField=%q is not a property on its own review form %s",
				agency.dir, agency.outcomeField, relPath(root, form.path))
		}
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return b
}

func readJSON(t *testing.T, path string, v any) error {
	t.Helper()
	return json.Unmarshal(readFile(t, path), v)
}

func relPath(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return r
}
