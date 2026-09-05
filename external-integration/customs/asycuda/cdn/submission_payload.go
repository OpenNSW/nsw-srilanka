package cdn

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/nswid"
)

// The SLC Edge CDN submission payload, per Annex B of the ASYCUDA ↔ NSW
// Interface Specification v1.3 (§7.1: POST api/cdn/v1, application/json, no
// attachments).
//
// The trader form (customs-cdn--user-form in one-trade-artifacts) is shaped
// around the printed Cargo Dispatch Note, which groups fields by where they sit
// on the paper form; Annex B is shaped around the ASYCUDA message. BuildPayload
// below is the translation between the two, and is the only place that needs to
// know either shape.
//
// One CDN carries exactly one container: Annex B's container block is a single
// object, not an array. A consignment of N containers is therefore N CDNs, each
// referencing the same declaration through cusDecRefs.
type Submission struct {
	Properties Properties `json:"properties"`

	OfficeCode string `json:"officeCode"`

	Shipper   Party `json:"shipper"`
	Consignee Party `json:"consignee"`

	// Voyage / transport.
	VoyageNumber       string `json:"voyageNumber"`
	VoyageDateAsString string `json:"voyageDateAsString"`
	Vessel             string `json:"vessel"`
	VesselOpCode       string `json:"vesselOpCode"`
	ContOpCode         string `json:"contOpCode"`
	DriverName         string `json:"driverName"`
	LorryRegNo         string `json:"lorryRegNo"`
	TrailerRegNo       string `json:"trailerRegNo"`
	LocationOfGoods    string `json:"locationOfGoods"`
	PlaceOfLoading     string `json:"placeOfLoading"`
	PlaceOfDischarge   string `json:"placeOfDischarge"`

	// Goods / packaging.
	GoodsDescription string  `json:"goodsDescription"`
	PackageNumber    int     `json:"packageNumber"`
	PackageType      string  `json:"packageType"`
	GrossWeight      float64 `json:"grossWeight"`
	Volume           float64 `json:"volume,omitempty"`
	BOL              string  `json:"bol,omitempty"`
	TempRequired     float64 `json:"tempRequired,omitempty"`

	// Container. Annex B marks all four mandatory.
	ContainerNumber string `json:"containerNumber"`
	ContainerType   string `json:"containerType"`
	SealNo          string `json:"sealNo"`
	ContainerMark   string `json:"containerMark"`

	// CusDecRefs is the one-to-many link to the declarations this note
	// dispatches cargo for (§7.1).
	CusDecRefs []DocumentReference `json:"cusDecRefs"`
}

// Party is Annex B's shipper / consignee block.
type Party struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	CountryCode string `json:"countryCode"`
}

// BuildPayload translates one trader CDN form submission into the Annex B
// payload.
//
// Missing optional values become their zero value rather than an error: the
// endpoint validates the note on integration and reports field-level problems
// through the §4.4 errors object, which produces a far better trader message
// than a local guess at what Customs will accept. The two things checked here
// are the ones the trader cannot see and cannot fix from an error message — an
// empty form, and a declaration reference that does not parse.
// Properties is Annex B's properties block: who is submitting, and the
// identifier that tells one logical submission from another (§2.2). See
// nswid.For for how that identifier is derived and why.
type Properties struct {
	Submitter string `json:"submitter"`
	NswID     string `json:"nswId"`
}

// submitterChannel identifies NSW as the submitting channel, as it does on the
// declaration.
const submitterChannel = "1"

func BuildPayload(form map[string]any, previousEdgeID string) (Submission, error) {
	if len(form) == 0 {
		return Submission{}, &buildError{"The dispatch note form could not be read."}
	}

	shipper := nested(form, "shipper")
	consignee := nested(form, "consignee")
	transport := nested(form, "transport")
	goods := nested(form, "goods")
	container := nested(form, "container")

	refs, err := buildCusDecRefs(form)
	if err != nil {
		return Submission{}, err
	}

	sub := Submission{
		Properties: Properties{Submitter: submitterChannel},
		OfficeCode: str(form, "officeCode"),
		Shipper: Party{
			ID:          str(shipper, "id"),
			Name:        str(shipper, "name"),
			Address:     str(shipper, "address"),
			CountryCode: str(shipper, "countryCode"),
		},
		Consignee: Party{
			ID:          str(consignee, "id"),
			Name:        str(consignee, "name"),
			Address:     str(consignee, "address"),
			CountryCode: str(consignee, "countryCode"),
		},
		VoyageNumber:       str(transport, "voyageNumber"),
		VoyageDateAsString: str(transport, "voyageDate"),
		Vessel:             str(transport, "vessel"),
		VesselOpCode:       str(transport, "vesselOpCode"),
		ContOpCode:         str(transport, "contOpCode"),
		DriverName:         str(transport, "driverName"),
		LorryRegNo:         str(transport, "lorryRegNo"),
		TrailerRegNo:       str(transport, "trailerRegNo"),
		LocationOfGoods:    str(transport, "locationOfGoods"),
		PlaceOfLoading:     str(transport, "placeOfLoading"),
		PlaceOfDischarge:   str(transport, "placeOfDischarge"),

		GoodsDescription: str(goods, "description"),
		PackageNumber:    integer(goods, "packageNumber"),
		PackageType:      str(goods, "packageType"),
		GrossWeight:      number(goods, "grossWeight"),
		Volume:           number(goods, "volume"),
		BOL:              str(goods, "bol"),
		TempRequired:     number(goods, "tempRequired"),

		ContainerNumber: str(container, "number"),
		ContainerType:   str(container, "type"),
		SealNo:          str(container, "sealNo"),
		ContainerMark:   str(container, "mark"),

		CusDecRefs: refs,
	}

	// Derived last, from the submission as it will be sent: the field is empty
	// while the digest is taken, so the identifier does not depend on itself.
	sub.Properties.NswID = nswid.For(sub, previousEdgeID)

	return sub, nil
}

// buildCusDecRefs resolves the declarations this note dispatches against.
//
// The form carries the registered reference as the display string the CusDec
// integration result produced ("CBEX1/2026/E/1047"), because that is what the
// workflow has to hand and what the trader sees on screen. Annex B needs it
// split back into the four canonical elements of §4.1.
func buildCusDecRefs(form map[string]any) ([]DocumentReference, error) {
	raw := form["cusDecRefs"]
	if raw == nil {
		// Single-reference forms carry the string directly.
		if s := str(form, "cusdecRef"); s != "" {
			ref, err := ParseDocumentReference(s)
			if err != nil {
				return nil, err
			}
			return []DocumentReference{ref}, nil
		}
		return nil, &buildError{"This dispatch note is not linked to a registered customs declaration."}
	}

	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil, &buildError{"This dispatch note is not linked to a registered customs declaration."}
	}

	refs := make([]DocumentReference, 0, len(list))
	for _, entry := range list {
		switch v := entry.(type) {
		case string:
			ref, err := ParseDocumentReference(v)
			if err != nil {
				return nil, err
			}
			refs = append(refs, ref)
		case map[string]any:
			refs = append(refs, DocumentReference{
				Office: str(v, "office"),
				Year:   str(v, "year"),
				Serial: str(v, "serial"),
				Number: integer(v, "number"),
			})
		default:
			return nil, &buildError{"A customs declaration reference on this dispatch note could not be read."}
		}
	}
	return refs, nil
}

// ParseDocumentReference splits the "office/year/serial/number" display form of
// a registered reference back into its four canonical elements (§4.1). That
// display form is what the CusDec integration result writes into the workflow,
// so it is what every downstream step — including this one — has to work from.
func ParseDocumentReference(s string) (DocumentReference, error) {
	parts := strings.Split(strings.TrimSpace(s), "/")
	if len(parts) != 4 {
		return DocumentReference{}, &buildError{fmt.Sprintf(
			"The customs declaration reference %q is not in the expected office/year/serial/number form.", s)}
	}

	number, err := strconv.Atoi(strings.TrimSpace(parts[3]))
	if err != nil {
		return DocumentReference{}, &buildError{fmt.Sprintf(
			"The customs declaration reference %q does not end in a declaration number.", s)}
	}

	ref := DocumentReference{
		Office: strings.TrimSpace(parts[0]),
		Year:   strings.TrimSpace(parts[1]),
		Serial: strings.TrimSpace(parts[2]),
		Number: number,
	}
	if !ref.IsValid() {
		return DocumentReference{}, &buildError{fmt.Sprintf(
			"The customs declaration reference %q is incomplete.", s)}
	}
	return ref, nil
}

// --- form accessors -------------------------------------------------------
//
// The form arrives as decoded JSON, so every value is any and every number is
// float64. These read through that without the caller repeating type
// assertions, returning the zero value for anything absent or the wrong shape.

func nested(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func number(m map[string]any, key string) float64 {
	switch n := m[key].(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case string:
		// JSONForms number widgets round-trip as strings in some browsers.
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}

func integer(m map[string]any, key string) int {
	return int(number(m, key))
}
