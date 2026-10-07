package cusdec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCusdecIntegrationResultRequest_DualFieldUnmarshaling(t *testing.T) {
	// Test live API format (event, processAt, cusDecRef)
	liveJSON := []byte(`{
		"event": "INTEGRATION_RESULT",
		"processAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cusDecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqLive CusdecIntegrationResultRequest
	err := json.Unmarshal(liveJSON, &reqLive)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqLive.Event)
	assert.Equal(t, "CBEX1", reqLive.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqLive.Payload.CusdecRef.Number)
	assert.NoError(t, reqLive.Validate())

	// Test spec prose format (eventType, processedAt, cusdecRef)
	specJSON := []byte(`{
		"eventType": "INTEGRATION_RESULT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"edgeId": "edge-123",
			"integrated": true,
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var reqSpec CusdecIntegrationResultRequest
	err = json.Unmarshal(specJSON, &reqSpec)
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION_RESULT", reqSpec.Event)
	assert.Equal(t, "CBEX1", reqSpec.Payload.CusdecRef.Office)
	assert.Equal(t, 43254, reqSpec.Payload.CusdecRef.Number)
	assert.NoError(t, reqSpec.Validate())
}

// §6.2 places edgeId, integrated, taxes, and errors inside payload.
func TestCusdecIntegrationResultRequest_NestedPayloadFields(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": true,
				"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
				"taxes": [{"code": "tax1", "rate": 1, "amount": 222}],
				"errors": {}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "CUSDEC_INTEGRATED", req.Event)
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.True(t, req.Integrated)
		assert.Len(t, req.Payload.Taxes, 1)
		assert.JSONEq(t, `{}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("failure carries payload errors", func(t *testing.T) {
		body := []byte(`{
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": false,
				"errors": {"Declaration.HSCode": ["Invalid HS code"]}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.False(t, req.Integrated)
		assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	// Only payload.* is authoritative: fields at the top level are not part of
	// §6.2 and must be ignored rather than silently correlating the wrong record.
	t.Run("top-level fields are ignored", func(t *testing.T) {
		body := []byte(`{
			"edgeId": "not-a-real-edge",
			"integrated": true,
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"errors": {"Bogus": ["ignored"]},
			"payload": {
				"edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
				"integrated": false,
				"errors": {"Declaration.HSCode": ["Invalid HS code"]}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.False(t, req.Integrated)
		assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
		assert.NoError(t, req.Validate())
	})

	t.Run("top-level only payload is rejected", func(t *testing.T) {
		body := []byte(`{
			"edgeId": "edge-legacy",
			"integrated": true,
			"eventType": "CUSDEC_INTEGRATED",
			"processedAt": "2026-06-26T04:04:52Z",
			"errors": {},
			"payload": {
				"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Empty(t, req.EdgeID)
		assert.False(t, req.Integrated)
		assert.EqualError(t, req.Validate(), "edgeId is required")
	})
}

// Decoding into a reused value must not carry fields the second document omits.
func TestCusdecIntegrationResultRequest_ReusedReceiverIsReset(t *testing.T) {
	var req CusdecIntegrationResultRequest

	first := []byte(`{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-06-26T04:04:52Z",
		"payload": {
			"edgeId": "edge-first",
			"integrated": true,
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"taxes": [{"code": "tax1", "rate": 1, "amount": 222}],
			"errors": {}
		}
	}`)
	require.NoError(t, json.Unmarshal(first, &req))
	require.Equal(t, "edge-first", req.EdgeID)
	require.True(t, req.Integrated)
	require.Len(t, req.Payload.Taxes, 1)

	second := []byte(`{
		"eventType": "CUSDEC_INTEGRATED",
		"processedAt": "2026-06-27T04:04:52Z",
		"payload": {
			"edgeId": "edge-second",
			"integrated": false,
			"errors": {"Declaration.HSCode": ["Invalid HS code"]}
		}
	}`)
	require.NoError(t, json.Unmarshal(second, &req))

	assert.Equal(t, "edge-second", req.EdgeID)
	assert.False(t, req.Integrated)
	assert.Empty(t, req.Payload.Taxes, "taxes from the first document must not survive")
	assert.False(t, req.Payload.CusdecRef.IsValid(), "cusdecRef from the first document must not survive")
	assert.JSONEq(t, `{"Declaration.HSCode": ["Invalid HS code"]}`, string(req.Errors))
}

func TestCusdecEventRequest_ReusedReceiverIsReset(t *testing.T) {
	var req CusdecEventRequest

	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "PAYMENT_CONFIRMED",
		"processedAt": "2026-04-26T11:15:22Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"amountPaid": 2035.00,
			"currency": "LKR"
		}
	}`), &req))
	require.Equal(t, 2035.00, req.Payload.AmountPaid)

	require.NoError(t, json.Unmarshal([]byte(`{
		"eventType": "EXPORT_RELEASED",
		"processedAt": "2026-04-26T11:15:22Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
			"vesselName": "EVER GIVEN"
		}
	}`), &req))

	assert.Equal(t, "EXPORT_RELEASED", req.Event)
	assert.Equal(t, "EVER GIVEN", req.Payload.VesselName)
	assert.Zero(t, req.Payload.AmountPaid, "amountPaid from the payment document must not survive")
	assert.Empty(t, req.Payload.Currency, "currency from the payment document must not survive")
}

func TestCusdecEventRequest_DualFieldUnmarshaling(t *testing.T) {
	specJSON := []byte(`{
		"eventType": "PAYMENT",
		"processedAt": "2026-07-20T05:46:05Z",
		"payload": {
			"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 43254}
		}
	}`)
	var req CusdecEventRequest
	err := json.Unmarshal(specJSON, &req)
	require.NoError(t, err)
	assert.Equal(t, "PAYMENT", req.Event)
	assert.Equal(t, "CBEX1", req.Payload.CusdecRef.Office)
	assert.NoError(t, req.Validate())
}

// --- spec v1.7 amountToPay ---------------------------------------------------

// §6.2 gained amountToPay in v1.7. It is the assessment, so it wins over any
// sum this side computes -- the spec's own example has the two disagree, 1254
// against tax lines totalling 1244.
func TestAmountToPay_PrefersWhatASYCUDASent(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(`{
      "eventType": "CUSDEC_INTEGRATED",
      "payload": {
        "edgeId": "5516e4c8-a93d-429d-8a18-6a484d331176",
        "integrated": true,
        "amountToPay": 1254,
        "taxes": [
          { "code": "tax1", "rate": 1, "amount": 222 },
          { "code": "tax2", "rate": 1, "amount": 1022 }
        ]
      }
    }`), &req))

	assert.Equal(t, float64(1254), amountToPay(req.Payload))
	assert.Equal(t, float64(1244), totalTaxes(req.Payload.Taxes),
		"the tax lines are still read; they are simply not the assessment")
}

// Every result sent against v1.6 carries no such field, and summing the lines
// stays the answer for those.
func TestAmountToPay_FallsBackToTheTaxLines(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(`{
      "payload": {
        "integrated": true,
        "taxes": [ { "code": "tax1", "rate": 1, "amount": 222 } ]
      }
    }`), &req))

	assert.Nil(t, req.Payload.AmountToPay)
	assert.Equal(t, float64(222), amountToPay(req.Payload))
}

// Nothing to pay is a real assessment, not a missing one, so an explicit zero
// must not fall through to the tax lines.
func TestAmountToPay_ZeroIsAnAnswer(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(`{
      "payload": {
        "integrated": true,
        "amountToPay": 0,
        "taxes": [ { "code": "tax1", "rate": 1, "amount": 222 } ]
      }
    }`), &req))

	require.NotNil(t, req.Payload.AmountToPay)
	assert.Equal(t, float64(0), amountToPay(req.Payload))
}

// --- spec v1.9 assessment ----------------------------------------------------

// specV19Integrated is the §6.2 CUSDEC_INTEGRATED example from spec v1.9, with
// its processedAt and the totalAssessedAmount its field table lists.
const specV19Integrated = `{
  "eventType": "CUSDEC_INTEGRATED",
  "processedAt": "2026-06-26T04:04:52Z",
  "payload": {
    "edgeId": "41d31df5-afab-40e8-bf32-1bec8180c0f6",
    "integrated": true,
    "nswId": "111222437000",
    "totalAssessedAmount": 1350.0,
    "amountPaid": 1350.0,
    "amountPayable": 0.0,
    "cusdecRef": { "number": 59, "office": "CBEX1", "serial": "E", "year": "2026" },
    "duties": {
      "globalDuties": [
        { "paymentMethodCode": "1", "taxAssessedAmount": 1100, "taxBaseAmount": 550, "taxRateNumeric": 2.0, "typeCode": "EPF" },
        { "paymentMethodCode": "1", "taxAssessedAmount": 250, "taxBaseAmount": 1, "taxRateNumeric": 250.0, "typeCode": "COM" }
      ],
      "itemDutiesList": [
        { "dutyTaxFees": [
            { "paymentMethodCode": "1", "taxAssessedAmount": 0, "taxBaseAmount": 125, "taxRateNumeric": 0.0, "typeCode": "CED" }
          ],
          "itemSequenceNumeric": 1 }
      ]
    },
    "errors": {},
    "status": "Paid"
  }
}`

// The v1.9 example reads in full: the reference, the three amounts, and both
// levels of the duties breakdown.
func TestIntegrationResult_ReadsTheV19Assessment(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(specV19Integrated), &req))
	require.NoError(t, req.Validate())

	assert.True(t, req.Integrated)
	assert.Equal(t, DocumentReference{Office: "CBEX1", Year: "2026", Serial: "E", Number: 59}, req.Payload.CusdecRef)
	require.NotNil(t, req.Payload.TotalAssessedAmount)
	require.NotNil(t, req.Payload.AmountPaid)
	require.NotNil(t, req.Payload.AmountPayable)
	assert.Equal(t, 1350.0, *req.Payload.TotalAssessedAmount)
	assert.Equal(t, 1350.0, *req.Payload.AmountPaid)
	assert.Equal(t, 0.0, *req.Payload.AmountPayable)

	require.Len(t, req.Payload.Duties.GlobalDuties, 2)
	assert.Equal(t, dutyLine{TypeCode: "EPF", TaxBaseAmount: 550, TaxRateNumeric: 2, TaxAssessedAmount: 1100, PaymentMethodCode: "1"},
		req.Payload.Duties.GlobalDuties[0])
	require.Len(t, req.Payload.Duties.ItemDutiesList, 1)
	assert.Equal(t, 1, req.Payload.Duties.ItemDutiesList[0].ItemSequenceNumeric)
	assert.Equal(t, "CED", req.Payload.Duties.ItemDutiesList[0].DutyTaxFees[0].TypeCode)
	assert.Equal(t, 1350.0, req.Payload.Duties.total())
}

// The example is a declaration already settled -- assessed 1350, paid 1350 --
// so the trader owes nothing. Charging the assessment instead would ask them
// to pay it twice.
func TestAmountToPay_V19ChargesWhatIsStillDue(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(specV19Integrated), &req))

	assert.Equal(t, 0.0, amountToPay(req.Payload))
}

func TestAmountToPay_V19Fallbacks(t *testing.T) {
	cases := map[string]struct {
		payload string
		want    float64
	}{
		"amountPayable wins over v1.7 amountToPay": {
			payload: `{"amountPayable": 900, "amountToPay": 1254}`,
			want:    900,
		},
		"no amountPayable: assessed less paid": {
			payload: `{"totalAssessedAmount": 1350.5, "amountPaid": 350.25}`,
			want:    1000.25,
		},
		"assessed with nothing paid yet": {
			payload: `{"totalAssessedAmount": 1350}`,
			want:    1350,
		},
		"only the duties breakdown: summed": {
			payload: `{"duties": {"globalDuties": [{"typeCode": "EPF", "taxAssessedAmount": 1100}],
			           "itemDutiesList": [{"itemSequenceNumeric": 1, "dutyTaxFees": [{"typeCode": "CED", "taxAssessedAmount": 0.1}]}]}}`,
			want: 1100.1,
		},
		"v1.7 amountToPay still read": {
			payload: `{"amountToPay": 1254, "taxes": [{"code": "tax1", "rate": 1, "amount": 222}]}`,
			want:    1254,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var p cusdecResultPayload
			require.NoError(t, json.Unmarshal([]byte(tc.payload), &p))
			assert.Equal(t, tc.want, amountToPay(p))
		})
	}
}
