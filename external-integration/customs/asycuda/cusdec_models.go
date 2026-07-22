package asycuda

import (
	"encoding/json"
	"errors"
	"time"
)

// CusdecStatus represents the lifecycle state of a Customs Declaration.
type CusdecStatus string

const (
	CusdecStatusSubmitted  CusdecStatus = "SUBMITTED"
	CusdecStatusIntegrated CusdecStatus = "INTEGRATED"
	CusdecStatusFailed     CusdecStatus = "FAILED"
)

// cusdecResultPayload is the nested "payload" object inside the §5
// callback. It carries the assigned CusDec reference on success.
type cusdecResultPayload struct {
	CusdecRef DocumentReference `json:"cusDecRef"`
}

// CusdecIntegrationResultRequest is the inbound DTO for the ASYCUDA §5 callback
// pushed when CusDec integration succeeds or fails.
type CusdecIntegrationResultRequest struct {
	// EdgeID is the correlation UUID saved when the CusDec was originally submitted.
	EdgeID string `json:"edgeId"`

	// Integrated is true when ASYCUDA successfully integrated the CusDec.
	Integrated bool `json:"integrated"`

	// Event is the callback event type: "INTEGRATION_RESULT".
	Event string `json:"event"`

	// ProcessAt is the timestamp when ASYCUDA processed the CusDec.
	ProcessAt time.Time `json:"processAt"`

	// Payload carries the CusDec reference assigned by customs on success.
	Payload cusdecResultPayload `json:"payload"`

	// Errors contains error details when integration fails. Keys and structure
	// are defined by ASYCUDA and vary by failure mode.
	Errors json.RawMessage `json:"errors,omitempty"`
}

// validate checks that a CusdecIntegrationResultRequest is well-formed.
func (r CusdecIntegrationResultRequest) validate() error {
	if r.EdgeID == "" {
		return errors.New("edgeId is required")
	}
	if r.Event == "" {
		return errors.New("event is required")
	}
	if r.ProcessAt.IsZero() {
		return errors.New("processAt is required")
	}
	// When integration succeeds the payload MUST carry a complete cusdecRef.
	if r.Integrated && !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusDecRef must be fully populated when integrated is true")
	}
	return nil
}

// CusdecDeclaration is the domain entity representing a Customs Declaration. It is
// created/updated when callbacks arrive.
type CusdecDeclaration struct {
	ID           string          `json:"id" gorm:"type:text;not null;primaryKey"`
	EdgeID       string          `json:"edge_id" gorm:"type:text;not null;uniqueIndex"` // Correlation UUID from submission
	Status       CusdecStatus    `json:"status" gorm:"type:text;not null;index"`
	CusdecYear   string          `json:"cusdec_year" gorm:"index:idx_cusdec_ref"`
	CusdecOffice string          `json:"cusdec_office" gorm:"index:idx_cusdec_ref"`
	CusdecSerial string          `json:"cusdec_serial" gorm:"index:idx_cusdec_ref"`
	CusdecNumber int             `json:"cusdec_number" gorm:"index:idx_cusdec_ref"`
	Errors       json.RawMessage `json:"errors" gorm:"type:jsonb"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
