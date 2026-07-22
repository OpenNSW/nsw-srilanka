package asycuda

// DocumentReference represents an ASYCUDA document reference (cdnRef or
// cusDecRef), a composite key that uniquely identifies a document within the
// customs system.
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
