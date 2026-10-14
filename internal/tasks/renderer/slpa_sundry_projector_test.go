package renderer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/OpenNSW/core/uiprojector"
)

// The sundry form as authored: a free-text service field on each line, which
// the projector turns into the list of services the CMS offers.
const sundryForm = `{
  "schema": {
    "type": "object",
    "properties": {
      "parentInvoiceNo": {"type": "string"},
      "lines": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "containerNo": {"type": "string"},
            "serviceId": {"type": "string", "title": "Sundry Service"},
            "quantity": {"type": "integer"}
          }
        }
      }
    }
  },
  "uiSchema": {"type": "VerticalLayout", "elements": []}
}`

func projectSundry(t *testing.T, record map[string]any) (uiprojector.FormContent, map[string]any) {
	t.Helper()

	projection, err := NewSLPASundryProjector().Project(context.Background(), []byte(sundryForm), record)
	require.NoError(t, err)

	form, ok := projection.Content.(uiprojector.FormContent)
	require.True(t, ok, "the projector renders a form")

	schema, ok := form.Schema.(map[string]any)
	require.True(t, ok)
	lines := schema["properties"].(map[string]any)["lines"].(map[string]any)
	field := lines["items"].(map[string]any)["properties"].(map[string]any)["serviceId"].(map[string]any)
	return form, field
}

func TestSundryProjector_OffersTheServicesTheCMSListed(t *testing.T) {
	_, field := projectSundry(t, map[string]any{
		"services": map[string]any{"services": []any{
			map[string]any{"service_id": "l1c73wk5", "service_name": "CRANE CHARGES OVER 10 M UP TO 15 M"},
			map[string]any{"service_id": "cn90sg2a", "service_name": "GARMENT"},
		}},
	})

	assert.Equal(t, []any{
		map[string]any{"const": "l1c73wk5", "title": "CRANE CHARGES OVER 10 M UP TO 15 M"},
		map[string]any{"const": "cn90sg2a", "title": "GARMENT"},
	}, field["oneOf"])
}

func TestSundryProjector_LeavesOutAnEntryMissingEitherHalf(t *testing.T) {
	_, field := projectSundry(t, map[string]any{
		"services": map[string]any{"services": []any{
			map[string]any{"service_id": "l1c73wk5"},
			map[string]any{"service_name": "GARMENT"},
			map[string]any{"service_id": "gq43znje", "service_name": "CRANE CHARGES OVER 15 M UP TO 20 M"},
		}},
	})

	assert.Equal(t, []any{
		map[string]any{"const": "gq43znje", "title": "CRANE CHARGES OVER 15 M UP TO 20 M"},
	}, field["oneOf"])
}

// With nothing to offer the field is left as authored: an empty oneOf is a field
// no line could satisfy, and the panel beside the form says what went wrong.
func TestSundryProjector_RendersTheFormAsAuthoredWithNothingToOffer(t *testing.T) {
	_, field := projectSundry(t, map[string]any{"services": map[string]any{"error": "SLPA could not list the sundry services:"}})

	assert.NotContains(t, field, "oneOf")
}

// The form is filled in from what the trader already entered, so a lookup run
// again does not lose their lines.
func TestSundryProjector_CarriesTheTradersEntries(t *testing.T) {
	entered := map[string]any{"parentInvoiceNo": "26211843262217"}
	form, _ := projectSundry(t, map[string]any{"sundryform": entered})

	assert.Equal(t, entered, form.Data)
}
