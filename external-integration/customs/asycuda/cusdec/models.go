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

type cusdecResultPayload struct {
	CusdecRef DocumentReference `json:"cusDecRef"`
}

func (p *cusdecResultPayload) UnmarshalJSON(data []byte) error {
	type Alias cusdecResultPayload
	aux := &struct {
		AltRef DocumentReference `json:"cusdecRef"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if !p.CusdecRef.IsValid() && aux.AltRef.IsValid() {
		p.CusdecRef = aux.AltRef
	}
	return nil
}

// CusdecIntegrationResultRequest is the inbound DTO for the ASYCUDA §5 callback
// pushed when CusDec integration succeeds or fails.
type CusdecIntegrationResultRequest struct {
	EdgeID     string              `json:"edgeId"`
	Integrated bool                `json:"integrated"`
	Event      string              `json:"event"`
	ProcessAt  time.Time           `json:"processAt"`
	Payload    cusdecResultPayload `json:"payload"`
	Errors     json.RawMessage     `json:"errors,omitempty"`
}

// UnmarshalJSON supports both live API fields (event, processAt) and spec fields (eventType, processedAt).
func (r *CusdecIntegrationResultRequest) UnmarshalJSON(data []byte) error {
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
	if r.Integrated && !r.Payload.CusdecRef.IsValid() {
		return errors.New("payload.cusDecRef must be fully populated when integrated is true")
	}
	return nil
}

type cusdecEventPayload struct {
	CusdecRef DocumentReference `json:"cusDecRef"`
}

func (p *cusdecEventPayload) UnmarshalJSON(data []byte) error {
	type Alias cusdecEventPayload
	aux := &struct {
		AltRef DocumentReference `json:"cusdecRef"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}
	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}
	if !p.CusdecRef.IsValid() && aux.AltRef.IsValid() {
		p.CusdecRef = aux.AltRef
	}
	return nil
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
		return errors.New("payload.cusDecRef must be fully populated")
	}
	return nil
}

// CusdecDeclaration is the domain entity representing a Customs Declaration.
type CusdecDeclaration struct {
	ID           string          `json:"id" gorm:"type:text;not null;primaryKey"`
	EdgeID       string          `json:"edge_id" gorm:"type:text;not null;uniqueIndex"`
	Status       CusdecStatus    `json:"status" gorm:"type:text;not null;index"`
	CusdecYear   string          `json:"cusdec_year" gorm:"index:idx_cusdec_ref"`
	CusdecOffice string          `json:"cusdec_office" gorm:"index:idx_cusdec_ref"`
	CusdecSerial string          `json:"cusdec_serial" gorm:"index:idx_cusdec_ref"`
	CusdecNumber int             `json:"cusdec_number" gorm:"index:idx_cusdec_ref"`
	Errors       json.RawMessage `json:"errors" gorm:"type:jsonb"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}
