package cusdec

import (
	"encoding/json"
	"errors"
	"time"
)

// DocumentReference represents an ASYCUDA document reference (cdnRef or cusDecRef).
type DocumentReference struct {
	Year   string `json:"year"`
	Office string `json:"office"`
	Serial string `json:"serial"`
	Number int    `json:"number"`
}

// IsValid reports whether all fields of the reference are populated and valid.
func (r DocumentReference) IsValid() bool {
	return r.Year != "" && r.Office != "" && r.Serial != "" && r.Number > 0
}

// CusdecStatus represents the lifecycle state of a Customs Declaration.
type CusdecStatus string

const (
	CusdecStatusSubmitted  CusdecStatus = "SUBMITTED"
	CusdecStatusIntegrated CusdecStatus = "INTEGRATED"
	CusdecStatusFailed     CusdecStatus = "FAILED"
	CusdecStatusPaid       CusdecStatus = "PAID"
	CusdecStatusWarranted  CusdecStatus = "WARRANTED"
	CusdecStatusReleased   CusdecStatus = "RELEASED"
)

// TaxEntry represents an assessed tax line item on a declaration (§6.2).
type TaxEntry struct {
	Code   string  `json:"code"`
	Rate   float64 `json:"rate"`
	Amount float64 `json:"amount"`
}

type cusdecResultPayload struct {
	CusdecRef  DocumentReference `json:"cusdecRef"`
	EdgeID     string            `json:"edgeId"`
	Integrated *bool             `json:"integrated"`
	Taxes      []TaxEntry        `json:"taxes,omitempty"`
	Errors     json.RawMessage   `json:"errors,omitempty"`
}

// CusdecIntegrationResultRequest is the inbound DTO for the ASYCUDA §6.2 callback
// pushed when CusDec integration succeeds or fails.
//
// Per §6.2, edgeId, integrated and errors travel inside payload; the fields below
// are hoisted out of it for the service layer and are never read from the top
// level of the request.
type CusdecIntegrationResultRequest struct {
	EdgeID     string              `json:"-"`
	Integrated bool                `json:"-"`
	Event      string              `json:"event"`
	ProcessAt  time.Time           `json:"processAt"`
	Payload    cusdecResultPayload `json:"payload"`
	Errors     json.RawMessage     `json:"-"`
}

// UnmarshalJSON supports both live API fields (event, processAt) and spec fields (eventType, processedAt).
func (r *CusdecIntegrationResultRequest) UnmarshalJSON(data []byte) error {
	// Clear the receiver so a reused value cannot retain fields the new document omits.
	*r = CusdecIntegrationResultRequest{}

	type Alias CusdecIntegrationResultRequest
	aux := &struct {
		EventType   string    `json:"eventType"`
		ProcessedAt time.Time `json:"processedAt"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if r.Event == "" && aux.EventType != "" {
		r.Event = aux.EventType
	}
	if r.ProcessAt.IsZero() && !aux.ProcessedAt.IsZero() {
		r.ProcessAt = aux.ProcessedAt
	}
	// §6.2 carries edgeId, integrated and errors inside payload, unlike the §6.5
	// notifications whose payload holds only cusdecRef and event-specific fields.
	r.EdgeID = r.Payload.EdgeID
	if r.Payload.Integrated != nil {
		r.Integrated = *r.Payload.Integrated
	}
	r.Errors = r.Payload.Errors
	return nil
}

func (r CusdecIntegrationResultRequest) Validate() error {
	if r.EdgeID == "" {
		return errors.New("edgeId is required")
	}
	if r.Event == "" {
		return errors.New("event is required")
	}
	if r.ProcessAt.IsZero() {
		return errors.New("processAt is required")
	}
	if r.Payload.Integrated == nil {
		return errors.New("integrated is required")
	}
	if r.Integrated && !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusdecRef must be fully populated when integrated is true")
	}

	// Validate payload.errors per §6.2 contract.
	if len(r.Payload.Errors) == 0 {
		return errors.New("payload.errors is required")
	}
	var errObj map[string]json.RawMessage
	if err := json.Unmarshal(r.Payload.Errors, &errObj); err != nil {
		return errors.New("payload.errors must be a JSON object")
	}
	if r.Integrated && len(errObj) != 0 {
		return errors.New("payload.errors must be empty ({}) when integrated is true")
	}
	if !r.Integrated && len(errObj) == 0 {
		return errors.New("payload.errors must contain at least one entry when integrated is false")
	}
	return nil
}

type cusdecEventPayload struct {
	CusdecRef DocumentReference `json:"cusdecRef"`

	// Payment Notification fields (§6.5.1)
	AmountPaid    float64 `json:"amountPaid,omitempty"`
	Currency      string  `json:"currency,omitempty"`
	BankReference string  `json:"bankReference,omitempty"`

	// Warranting Notification fields (§6.5.2)
	ReleaseOrderNo      string `json:"releaseOrderNo,omitempty"`
	ExaminationRequired bool   `json:"examinationRequired,omitempty"`

	// Export Release fields (§6.5.3)
	VesselName    string `json:"vesselName,omitempty"`
	VoyageNo      string `json:"voyageNo,omitempty"`
	PortOfLoading string `json:"portOfLoading,omitempty"`
}

// CusdecEventRequest is the inbound DTO for ASYCUDA lifecycle event callbacks
// (PAYMENT, WARRANTING, RELEASE).
type CusdecEventRequest struct {
	Event     string             `json:"event"`
	ProcessAt time.Time          `json:"processAt"`
	Payload   cusdecEventPayload `json:"payload"`
}

// UnmarshalJSON supports both live API fields (event, processAt) and spec fields (eventType, processedAt).
func (r *CusdecEventRequest) UnmarshalJSON(data []byte) error {
	// Clear the receiver so a reused value cannot retain fields the new document omits.
	*r = CusdecEventRequest{}

	type Alias CusdecEventRequest
	aux := &struct {
		EventType   string    `json:"eventType"`
		ProcessedAt time.Time `json:"processedAt"`
		*Alias
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if r.Event == "" && aux.EventType != "" {
		r.Event = aux.EventType
	}
	if r.ProcessAt.IsZero() && !aux.ProcessedAt.IsZero() {
		r.ProcessAt = aux.ProcessedAt
	}
	return nil
}

func (r CusdecEventRequest) Validate() error {
	if r.Event == "" {
		return errors.New("event is required")
	}
	if r.ProcessAt.IsZero() {
		return errors.New("processAt is required")
	}
	if !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusdecRef must be fully populated")
	}
	return nil
}

// CusdecDeclaration is the domain entity representing a Customs Declaration.
type CusdecDeclaration struct {
	ID           string          `json:"id" gorm:"type:text;not null;primaryKey"`
	EdgeID       string          `json:"edge_id" gorm:"type:text;not null;uniqueIndex"`
	Status       CusdecStatus    `json:"status" gorm:"type:text;not null;index"`
	CusdecYear   string          `json:"cusdec_year" gorm:"column:cusdec_year;index:idx_cusdec_ref"`
	CusdecOffice string          `json:"cusdec_office" gorm:"column:cusdec_office;index:idx_cusdec_ref"`
	CusdecSerial string          `json:"cusdec_serial" gorm:"column:cusdec_serial;index:idx_cusdec_ref"`
	CusdecNumber int             `json:"cusdec_number" gorm:"column:cusdec_number;index:idx_cusdec_ref"`
	Errors       json.RawMessage `json:"errors" gorm:"type:jsonb"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
