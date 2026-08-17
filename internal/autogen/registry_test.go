package autogen

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleYAML = `
issuers:
  - issuer: RTA
    formats:
      - idType: application_id
        segments:
          - type: literal
            value: "RTA-APP-"
          - type: list
            list: office_location
            param: officeCode
          - type: literal
            value: "-"
          - type: date
            layout: "20060102"
          - type: literal
            value: "-"
          - type: sequence
            scopeKey: "{issuer}:{idType}:{officeCode}:{yyyyMMdd}"
            padding: 6

      - idType: permit_id
        segments:
          - type: literal
            value: "RTA-PMT-"
          - type: list
            list: office_location
            param: officeCode
          - type: literal
            value: "-"
          - type: sequence
            scopeKey: "{issuer}:{idType}:{officeCode}"
            padding: 8

lists:
  office_location: [COL, GAL, KAN]
`

func TestRegistry_LoadFromYAML(t *testing.T) {
	reg := NewRegistry()
	err := reg.LoadFromYAML([]byte(sampleYAML))
	require.NoError(t, err)

	// Validate list
	require.NoError(t, reg.ValidateListParam("office_location", "COL"))
	require.NoError(t, reg.ValidateListParam("office_location", "GAL"))
	require.Error(t, reg.ValidateListParam("office_location", "INVALID"))

	// Retrieve application_id format
	fmtCfg, found := reg.GetFormat("RTA", "application_id")
	require.True(t, found)
	assert.Equal(t, "application_id", fmtCfg.IDType)
	assert.Len(t, fmtCfg.Segments, 6)

	// Retrieve permit_id format
	fmtCfgPermit, found := reg.GetFormat("rta", "PERMIT_ID")
	require.True(t, found)
	assert.Equal(t, "permit_id", fmtCfgPermit.IDType)
	assert.Len(t, fmtCfgPermit.Segments, 4)
}
