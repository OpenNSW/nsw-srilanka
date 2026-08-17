package autogen

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrIssuerRequired = errors.New("autogen: issuer is required")
)

var placeholderRegexp = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// ReferenceSequence maps to database table reference_sequences.
type ReferenceSequence struct {
	ScopeKey     string    `gorm:"primaryKey;column:scope_key"`
	CurrentValue int64     `gorm:"column:current_value"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (ReferenceSequence) TableName() string {
	return "reference_sequences"
}

// SequenceService provides methods to generate atomic, segment-based reference numbers.
type SequenceService interface {
	GenerateID(ctx context.Context, issuer, idType string, params map[string]string) (string, error)
	GetOrGenerateID(ctx context.Context, rootWorkflowID, parentWorkflowID, issuer, idType, targetKey string, params map[string]string) (string, error)
	GenerateNext(ctx context.Context, agencyCode string) (string, error)
	GetOrGenerateNext(ctx context.Context, rootWorkflowID, parentWorkflowID, agencyCode, targetKey string) (string, error)
}

type sequenceService struct {
	db       *gorm.DB
	registry *Registry
}

// NewSequenceService creates a new sequence generation service.
func NewSequenceService(db *gorm.DB, registry *Registry) SequenceService {
	if registry == nil {
		registry = NewRegistry()
	}
	return &sequenceService{db: db, registry: registry}
}

// GenerateID builds a reference number for the specified issuer and idType using segment definitions.
func (s *sequenceService) GenerateID(ctx context.Context, issuer, idType string, params map[string]string) (string, error) {
	if issuer == "" {
		return "", ErrIssuerRequired
	}
	if idType == "" {
		idType = "application_id"
	}
	if params == nil {
		params = make(map[string]string)
	}

	format, found := s.registry.GetFormat(issuer, idType)
	if !found {
		// Fallback format if (issuer, idType) is not explicitly registered
		format = FormatConfig{
			IDType: idType,
			Segments: []Segment{
				{Type: SegmentLiteral, Value: strings.ToUpper(issuer) + "-"},
				{Type: SegmentDate, Layout: "20060102"},
				{Type: SegmentLiteral, Value: "-"},
				{Type: SegmentSequence, ScopeKey: "{issuer}:{idType}:{yyyyMMdd}", Padding: 5},
			},
		}
	}

	now := time.Now().UTC()
	var builder strings.Builder

	for _, seg := range format.Segments {
		switch seg.Type {
		case SegmentLiteral:
			builder.WriteString(seg.Value)

		case SegmentList:
			paramVal, ok := params[seg.Param]
			if !ok || strings.TrimSpace(paramVal) == "" {
				return "", fmt.Errorf("autogen: missing required parameter %q for list %q", seg.Param, seg.List)
			}
			if err := s.registry.ValidateListParam(seg.List, paramVal); err != nil {
				return "", err
			}
			builder.WriteString(strings.TrimSpace(paramVal))

		case SegmentDate:
			layout := seg.Layout
			if layout == "" {
				layout = "20060102"
			}
			builder.WriteString(now.Format(layout))

		case SegmentSequence:
			scopeKey := resolveScopeKey(seg.ScopeKey, issuer, idType, now, params)
			val, err := s.incrementSequence(ctx, scopeKey)
			if err != nil {
				return "", fmt.Errorf("autogen: sequence counter %q: %w", scopeKey, err)
			}
			padding := seg.Padding
			if padding <= 0 {
				padding = 5
			}
			builder.WriteString(fmt.Sprintf("%0*d", padding, val))

		default:
			return "", fmt.Errorf("autogen: unsupported segment type %q", seg.Type)
		}
	}

	return builder.String(), nil
}

// GetOrGenerateID checks task_records_v2 for an existing reference ID in the workflow under targetKey.
// If found, it returns the existing ID; otherwise, it generates a new ID.
func (s *sequenceService) GetOrGenerateID(ctx context.Context, rootWorkflowID, parentWorkflowID, issuer, idType, targetKey string, params map[string]string) (string, error) {
	if targetKey == "" {
		targetKey = "reference_number"
	}

	if s.db != nil && (rootWorkflowID != "" || parentWorkflowID != "") {
		var existingRef string
		query := s.db.WithContext(ctx).Table("task_records_v2").Select("data->>?", targetKey)
		if rootWorkflowID != "" {
			query = query.Where("root_workflow_id = ?", rootWorkflowID)
		} else {
			query = query.Where("parent_workflow_id = ?", parentWorkflowID)
		}

		err := query.Where("data->>? IS NOT NULL AND data->>? != ''", targetKey, targetKey).Limit(1).Scan(&existingRef).Error
		if err == nil && existingRef != "" {
			return existingRef, nil
		}
	}

	return s.GenerateID(ctx, issuer, idType, params)
}

// GenerateNext wraps GenerateID for backwards compatibility with single agency code calls.
func (s *sequenceService) GenerateNext(ctx context.Context, agencyCode string) (string, error) {
	return s.GenerateID(ctx, agencyCode, "application_id", nil)
}

// GetOrGenerateNext wraps GetOrGenerateID for backwards compatibility.
func (s *sequenceService) GetOrGenerateNext(ctx context.Context, rootWorkflowID, parentWorkflowID, agencyCode, targetKey string) (string, error) {
	return s.GetOrGenerateID(ctx, rootWorkflowID, parentWorkflowID, agencyCode, "application_id", targetKey, nil)
}

func (s *sequenceService) incrementSequence(ctx context.Context, scopeKey string) (int64, error) {
	var nextVal int64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var seq ReferenceSequence
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("scope_key = ?", scopeKey).First(&seq).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				seq = ReferenceSequence{
					ScopeKey:     scopeKey,
					CurrentValue: 0,
				}
			} else {
				return err
			}
		}
		seq.CurrentValue++
		seq.UpdatedAt = time.Now().UTC()
		if err := tx.Save(&seq).Error; err != nil {
			return err
		}
		nextVal = seq.CurrentValue
		return nil
	})
	return nextVal, err
}

func resolveScopeKey(template, issuer, idType string, t time.Time, params map[string]string) string {
	if template == "" {
		template = "{issuer}:{idType}"
	}

	result := placeholderRegexp.ReplaceAllStringFunc(template, func(match string) string {
		key := strings.Trim(match, "{}")
		switch key {
		case "issuer":
			return strings.ToUpper(issuer)
		case "idType":
			return strings.ToLower(idType)
		case "yyyy":
			return t.Format("2006")
		case "yyyyMM":
			return t.Format("200601")
		case "yyyyMMdd":
			return t.Format("20060102")
		default:
			if v, ok := params[key]; ok {
				return strings.TrimSpace(v)
			}
			return match
		}
	})
	return result
}
