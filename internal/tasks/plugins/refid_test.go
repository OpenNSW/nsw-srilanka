package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/OpenNSW/core/refid"
	"github.com/OpenNSW/core/taskflow/plugins"
	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRefIDs records the last Generate call and returns id or err.
type fakeRefIDs struct {
	id     string
	err    error
	calls  int
	issuer string
	idType string
	params map[string]string
}

func (f *fakeRefIDs) Generate(_ context.Context, issuer, idType string, params map[string]string) (string, error) {
	f.calls++
	f.issuer, f.idType, f.params = issuer, idType, params
	return f.id, f.err
}

// memSequences is an in-memory refid.SequenceStore.
type memSequences struct {
	mu       sync.Mutex
	counters map[string]int64
}

func (m *memSequences) Next(_ context.Context, scopeKey string, maxValue int64) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.counters[scopeKey] >= maxValue {
		return 0, refid.ErrCounterOverflow
	}
	m.counters[scopeKey]++
	return m.counters[scopeKey], nil
}

const refIDProps = `{"issuer": "TNSW", "id_type": "consignment_ref"}`

func refIDCtx(inputs map[string]any) plugins.PluginContext {
	return plugins.PluginContext{
		Context:         context.Background(),
		Inputs:          inputs,
		Record:          &store.TaskRecord{TaskID: "task-1", Data: map[string]any{}},
		OutputNamespace: "refid",
	}
}

func TestRefIDGenerator_WritesIDToNamespace(t *testing.T) {
	reg := &fakeRefIDs{id: "TNSW-000001"}
	ctx := refIDCtx(map[string]any{"portCode": "CMB"})

	require.NoError(t, NewRefIDGeneratorPlugin(reg).Execute(ctx, json.RawMessage(refIDProps)))

	assert.Equal(t, map[string]any{"reference_id": "TNSW-000001"}, ctx.Record.Data["refid"])
	assert.Equal(t, "TNSW", reg.issuer)
	assert.Equal(t, "consignment_ref", reg.idType)
}

func TestRefIDGenerator_PassesOnlyStringInputsAsParams(t *testing.T) {
	reg := &fakeRefIDs{id: "X"}
	ctx := refIDCtx(map[string]any{
		"portCode": "CMB",
		"count":    float64(3),
		"userform": map[string]any{"portCode": "HBT"},
	})

	require.NoError(t, NewRefIDGeneratorPlugin(reg).Execute(ctx, json.RawMessage(refIDProps)))

	assert.Equal(t, map[string]string{"portCode": "CMB"}, reg.params)
}

func TestRefIDGenerator_ReusesExistingID(t *testing.T) {
	reg := &fakeRefIDs{id: "TNSW-000002"}
	ctx := refIDCtx(nil)
	ctx.Record.Data["refid"] = map[string]any{"reference_id": "TNSW-000001"}

	require.NoError(t, NewRefIDGeneratorPlugin(reg).Execute(ctx, json.RawMessage(refIDProps)))

	assert.Zero(t, reg.calls, "a task that already holds an ID must not generate another")
	assert.Equal(t, map[string]any{"reference_id": "TNSW-000001"}, ctx.Record.Data["refid"])
}

func TestRefIDGenerator_NilDataInitialised(t *testing.T) {
	ctx := refIDCtx(nil)
	ctx.Record.Data = nil

	require.NoError(t, NewRefIDGeneratorPlugin(&fakeRefIDs{id: "X"}).Execute(ctx, json.RawMessage(refIDProps)))

	assert.Equal(t, map[string]any{"reference_id": "X"}, ctx.Record.Data["refid"])
}

func TestRefIDGenerator_Rejected(t *testing.T) {
	cases := []struct {
		name  string
		props string
		mut   func(*plugins.PluginContext)
	}{
		{name: "malformed properties", props: `{`},
		{name: "missing issuer", props: `{"id_type": "consignment_ref"}`},
		{name: "blank issuer", props: `{"issuer": " ", "id_type": "consignment_ref"}`},
		{name: "missing id_type", props: `{"issuer": "TNSW"}`},
		{name: "missing output namespace", props: refIDProps, mut: func(c *plugins.PluginContext) { c.OutputNamespace = "" }},
		{name: "nil record", props: refIDProps, mut: func(c *plugins.PluginContext) { c.Record = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reg := &fakeRefIDs{id: "X"}
			ctx := refIDCtx(nil)
			if tc.mut != nil {
				tc.mut(&ctx)
			}
			require.Error(t, NewRefIDGeneratorPlugin(reg).Execute(ctx, json.RawMessage(tc.props)))
			assert.Zero(t, reg.calls)
		})
	}
}

func TestRefIDGenerator_GenerateErrorKeepsSentinel(t *testing.T) {
	reg := &fakeRefIDs{err: fmt.Errorf("%w: portCode missing", refid.ErrInvalidParam)}
	ctx := refIDCtx(nil)

	err := NewRefIDGeneratorPlugin(reg).Execute(ctx, json.RawMessage(refIDProps))

	require.ErrorIs(t, err, refid.ErrInvalidParam)
	assert.NotContains(t, ctx.Record.Data, "refid")
}

func TestNewRefIDGeneratorPlugin_NilRegistryPanics(t *testing.T) {
	assert.Panics(t, func() { NewRefIDGeneratorPlugin(nil) })
}

// Runs a real refid registry end to end, so the plugin's params reach the
// format the way a configured deployment would use them.
func TestRefIDGenerator_RealRegistry(t *testing.T) {
	reg, err := refid.NewRegistry(refid.Config{
		Issuers: []refid.IssuerConfig{{
			Issuer: "TNSW",
			Formats: []refid.FormatConfig{{
				IDType: "consignment_ref",
				Segments: []refid.SegmentConfig{
					{Type: refid.SegmentTypeLiteral, Value: "TNSW-"},
					{Type: refid.SegmentTypeList, List: "port", Param: "portCode"},
					{Type: refid.SegmentTypeLiteral, Value: "-"},
					{Type: refid.SegmentTypeSequence, Sequence: &refid.SequenceSegmentConfig{
						ScopeKey: "{issuer}:{idType}:{portCode}",
						Padding:  6,
					}},
				},
			}},
		}},
		Lists: map[string][]string{"port": {"CMB", "HBT"}},
	}, refid.WithSequenceStore(&memSequences{counters: map[string]int64{}}))
	require.NoError(t, err)
	plugin := NewRefIDGeneratorPlugin(reg)

	generate := func(port string) (string, error) {
		ctx := refIDCtx(map[string]any{"portCode": port})
		if err := plugin.Execute(ctx, json.RawMessage(refIDProps)); err != nil {
			return "", err
		}
		return ctx.Record.Data["refid"].(map[string]any)["reference_id"].(string), nil
	}

	for _, want := range []string{"TNSW-CMB-000001", "TNSW-CMB-000002"} {
		got, err := generate("CMB")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	}
	got, err := generate("HBT")
	require.NoError(t, err)
	assert.Equal(t, "TNSW-HBT-000001", got, "each port counts independently")

	_, err = generate("GAL")
	assert.True(t, errors.Is(err, refid.ErrInvalidParam), "a port outside the list must be rejected, got %v", err)
}
