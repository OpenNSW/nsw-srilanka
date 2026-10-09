package cusdec

import (
	"encoding/json"
	"fmt"
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
			"totalAssessedAmount": 1244,
			"amountPayable": 1244,
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
			"totalAssessedAmount": 1244,
			"amountPayable": 1244,
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
				"totalAssessedAmount": 1244,
				"cusdecRef": {"year": "2026", "office": "CBEX1", "serial": "E", "number": 1047},
				"amountPayable": 1100,
				"duties": {"globalDuties": [{"typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1"}]},
				"errors": {}
			}
		}`)
		var req CusdecIntegrationResultRequest
		require.NoError(t, json.Unmarshal(body, &req))
		assert.Equal(t, "CUSDEC_INTEGRATED", req.Event)
		assert.Equal(t, "5516e4c8-a93d-429d-8a18-6a484d331176", req.EdgeID)
		assert.True(t, req.Integrated)
		assert.Len(t, req.Payload.Duties.GlobalDuties, 1)
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
			"duties": {"globalDuties": [{"typeCode": "EPF", "taxBaseAmount": 550, "taxRateNumeric": 2.0, "taxAssessedAmount": 1100, "paymentMethodCode": "1"}]},
			"errors": {}
		}
	}`)
	require.NoError(t, json.Unmarshal(first, &req))
	require.Equal(t, "edge-first", req.EdgeID)
	require.True(t, req.Integrated)
	require.Len(t, req.Payload.Duties.GlobalDuties, 1)

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
	assert.Empty(t, req.Payload.Duties.GlobalDuties, "duties from the first document must not survive")
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
	assert.Equal(t, itemNumber(1), req.Payload.Duties.ItemDutiesList[0].ItemSequenceNumeric)
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

// amountPayable is the only source: the parts it is computed from and the
// duties breakdown are not used to reconstruct it.
func TestAmountToPay_ReadsOnlyAmountPayable(t *testing.T) {
	cases := map[string]struct {
		payload string
		want    float64
	}{
		"amountPayable is charged": {
			payload: `{"amountPayable": 900, "totalAssessedAmount": 1350, "amountPaid": 0}`,
			want:    900,
		},
		"assessed and paid without amountPayable: not reconstructed": {
			payload: `{"totalAssessedAmount": 1350, "amountPaid": 350}`,
			want:    0,
		},
		"duties without amountPayable: not summed": {
			payload: `{"duties": {"globalDuties": [{"typeCode": "EPF", "taxAssessedAmount": 1100}]}}`,
			want:    0,
		},
		"nothing stated": {
			payload: `{}`,
			want:    0,
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

// A successful result must state its assessment. Without amountPayable it
// would read as owing nothing and skip the payment step, so it is refused.
func TestIntegrationResult_SuccessMustStateItsAssessment(t *testing.T) {
	base := `{"eventType": "CUSDEC_INTEGRATED", "processedAt": "2026-06-26T04:04:52Z",
	  "payload": {"edgeId": "e1", "integrated": true,
	    "cusdecRef": {"office": "CBEX1", "year": "2026", "serial": "E", "number": 59}%s}}`
	cases := map[string]struct {
		amounts string
		wantErr string
	}{
		"both stated":            {`, "totalAssessedAmount": 1350, "amountPayable": 0`, ""},
		"no amountPayable":       {`, "totalAssessedAmount": 1350`, "payload.amountPayable is required when integrated is true"},
		"no totalAssessedAmount": {`, "amountPayable": 1000`, "payload.totalAssessedAmount is required when integrated is true"},
		"neither":                {``, "payload.amountPayable is required when integrated is true"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var req CusdecIntegrationResultRequest
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(base, tc.amounts)), &req))
			if tc.wantErr == "" {
				assert.NoError(t, req.Validate())
			} else {
				assert.EqualError(t, req.Validate(), tc.wantErr)
			}
		})
	}
}

// A rejection assesses nothing, so it needs no amounts.
func TestIntegrationResult_RejectionNeedsNoAssessment(t *testing.T) {
	var req CusdecIntegrationResultRequest
	require.NoError(t, json.Unmarshal([]byte(`{"eventType": "CUSDEC_INTEGRATED", "processedAt": "2026-06-26T04:04:52Z",
	  "payload": {"edgeId": "e1", "integrated": false, "errors": {"0": [{"code": 410, "description": "Missing HS code"}]}}}`), &req))
	assert.NoError(t, req.Validate())
}

// The spec says duties is "empty when integration failed" without saying how.
// Every empty form is read as no duties, so the rejection still reaches the
// trader rather than failing the whole callback.
func TestIntegrationResult_FailureWithEmptyDutiesInAnyForm(t *testing.T) {
	for _, duties := range []string{`[]`, `{}`, `null`} {
		t.Run(duties, func(t *testing.T) {
			var req CusdecIntegrationResultRequest
			require.NoError(t, json.Unmarshal([]byte(fmt.Sprintf(`{"eventType": "CUSDEC_INTEGRATED", "processedAt": "2026-06-26T04:04:52Z",
			  "payload": {"edgeId": "e1", "integrated": false, "duties": %s,
			    "errors": {"0": [{"code": 410, "description": "Missing HS code"}]}}}`, duties)), &req))
			assert.False(t, req.Integrated)
			assert.Empty(t, req.Payload.Duties.GlobalDuties)
			assert.Empty(t, req.Payload.Duties.ItemDutiesList)
			assert.NoError(t, req.Validate())
		})
	}
}

// A non-empty array cannot be the breakdown, so it is still refused.
func TestIntegrationResult_DutiesAsANonEmptyArrayIsRefused(t *testing.T) {
	var p cusdecResultPayload
	err := json.Unmarshal([]byte(`{"duties": [{"typeCode": "EPF"}]}`), &p)
	assert.ErrorContains(t, err, "duties: expected an object")
}

// A whole item number written as 1.0 is read as 1; a fraction cannot name an
// item and is refused.
func TestIntegrationResult_ItemSequenceNumericAcceptsAWholeFloat(t *testing.T) {
	var p cusdecResultPayload
	require.NoError(t, json.Unmarshal([]byte(`{"duties": {"itemDutiesList": [{"itemSequenceNumeric": 1.0, "dutyTaxFees": []}]}}`), &p))
	assert.Equal(t, itemNumber(1), p.Duties.ItemDutiesList[0].ItemSequenceNumeric)

	err := json.Unmarshal([]byte(`{"duties": {"itemDutiesList": [{"itemSequenceNumeric": 1.5}]}}`), &p)
	assert.ErrorContains(t, err, "is not a whole number")
}
