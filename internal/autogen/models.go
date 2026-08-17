package autogen

// SegmentType identifies the type of a reference ID segment.
type SegmentType string

const (
	SegmentLiteral  SegmentType = "literal"
	SegmentList     SegmentType = "list"
	SegmentDate     SegmentType = "date"
	SegmentSequence SegmentType = "sequence"
)

// Segment defines a single building block of a reference ID format.
type Segment struct {
	Type     SegmentType `yaml:"type" json:"type"`
	Value    string      `yaml:"value,omitempty" json:"value,omitempty"`       // For literal
	List     string      `yaml:"list,omitempty" json:"list,omitempty"`         // For list: list name
	Param    string      `yaml:"param,omitempty" json:"param,omitempty"`       // For list: caller param name
	Layout   string      `yaml:"layout,omitempty" json:"layout,omitempty"`     // For date: e.g. "20060102"
	ScopeKey string      `yaml:"scopeKey,omitempty" json:"scopeKey,omitempty"` // For sequence: e.g. "{issuer}:{idType}:{officeCode}:{yyyyMMdd}"
	Padding  int         `yaml:"padding,omitempty" json:"padding,omitempty"`   // For sequence: e.g. 6
}

// FormatConfig defines the segments for a specific ID type under an issuer.
type FormatConfig struct {
	IDType   string    `yaml:"idType" json:"idType"`
	Segments []Segment `yaml:"segments" json:"segments"`
}

// IssuerConfig holds all ID formats defined for a specific agency/issuer.
type IssuerConfig struct {
	Issuer  string         `yaml:"issuer" json:"issuer"`
	Formats []FormatConfig `yaml:"formats" json:"formats"`
}

// RegistryConfig represents the root configuration structure for reference ID formats.
type RegistryConfig struct {
	Issuers []IssuerConfig      `yaml:"issuers" json:"issuers"`
	Lists   map[string][]string `yaml:"lists" json:"lists"`
}
