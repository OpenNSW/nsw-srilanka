package cdn

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

// DispatchNoteStatus represents the lifecycle state of a Cargo Dispatch Note.
type DispatchNoteStatus string

const (
	DispatchNoteStatusSubmitted    DispatchNoteStatus = "SUBMITTED"
	DispatchNoteStatusIntegrated   DispatchNoteStatus = "INTEGRATED"
	DispatchNoteStatusFailed       DispatchNoteStatus = "FAILED"
	DispatchNoteStatusAcknowledged DispatchNoteStatus = "ACKNOWLEDGED"
)

// --------------------------------------------------------
// §7.2 — CDN Integration Result callback DTO
// --------------------------------------------------------

type integrationResultPayload struct {
	CDNRef DocumentReference `json:"cdnRef"`
}

// CDNIntegrationResultRequest is the inbound DTO for the ASYCUDA §7.2 callback
// pushed when CDN integration succeeds or fails.
type CDNIntegrationResultRequest struct {
	EdgID      string                   `json:"edgId"`
	Integrated bool                     `json:"integrated"`
	Event      string                   `json:"event"`
	ProcessAt  time.Time                `json:"processAt"`
	Payload    integrationResultPayload `json:"payload"`
	Errors     json.RawMessage          `json:"errors,omitempty"`
}

// UnmarshalJSON supports both live API fields (event, processAt) and spec fields (eventType, processedAt).
func (r *CDNIntegrationResultRequest) UnmarshalJSON(data []byte) error {
	type Alias CDNIntegrationResultRequest
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

func (r CDNIntegrationResultRequest) Validate() error {
	if r.EdgID == "" {
		return errors.New("edgId is required")
	}
	if r.Event == "" {
		return errors.New("event is required")
	}
	if r.ProcessAt.IsZero() {
		return errors.New("processAt is required")
	}
	if r.Integrated && !r.Payload.CDNRef.IsValid() {
		return errors.New("payload.cdnRef must be fully populated when integrated is true")
	}
	return nil
}

// --------------------------------------------------------
// §7.3 — CDN Acknowledgment callback DTO
// --------------------------------------------------------

type acknowledgmentPayload struct {
	CDNRef DocumentReference `json:"cdnRef"`
}

// CDNAcknowledgmentRequest is the inbound DTO for the ASYCUDA §7.3 callback
// pushed as a notification after the CDN has been acknowledged.
type CDNAcknowledgmentRequest struct {
	Event     string                `json:"event"`
	ProcessAt time.Time             `json:"processAt"`
	Payload   acknowledgmentPayload `json:"payload"`
}

// UnmarshalJSON supports both live API fields (event, processAt) and spec fields (eventType, processedAt).
func (r *CDNAcknowledgmentRequest) UnmarshalJSON(data []byte) error {
	type Alias CDNAcknowledgmentRequest
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

func (r CDNAcknowledgmentRequest) Validate() error {
	if r.Event == "" {
		return errors.New("event is required")
	}
	if r.ProcessAt.IsZero() {
		return errors.New("processAt is required")
	}
	if !r.Payload.CDNRef.IsValid() {
		return errors.New("payload.cdnRef must be fully populated")
	}
	return nil
}

// --------------------------------------------------------
// Domain entity
// --------------------------------------------------------

// DispatchNote is the domain entity representing a Cargo Dispatch Note.
type DispatchNote struct {
	ID        string             `json:"id" gorm:"type:text;not null;primaryKey"`
	EdgID     string             `json:"edg_id" gorm:"type:text;not null;uniqueIndex"`
	Status    DispatchNoteStatus `json:"status" gorm:"type:text;not null;index"`
	CDNYear   string             `json:"cdn_year" gorm:"column:cdn_year;index:idx_cdn_ref"`
	CDNOffice string             `json:"cdn_office" gorm:"column:cdn_office;index:idx_cdn_ref"`
	CDNSerial string             `json:"cdn_serial" gorm:"column:cdn_serial;index:idx_cdn_ref"`
	CDNNumber int                `json:"cdn_number" gorm:"column:cdn_number;index:idx_cdn_ref"`
	Errors    json.RawMessage    `json:"errors" gorm:"type:jsonb"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
}
