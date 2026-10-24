// Package boatnote prefills the boat note application from what Sri Lanka
// Customs holds for the consignment: the declaration as it stands in ASYCUDA
// World, and the Cargo Dispatch Notes registered against it.
package boatnote

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/cdn"
)

// SLC Edge's retrieve endpoints (§6.7, §7.8).
const (
	DeclarationRetrievePath = "/api/declaration/v1/retrieve"
	CDNRetrievePath         = "/api/cdn/v1/retrieve"
)

// Caller posts a JSON body to an SLC Edge path and returns the decoded answer.
type Caller interface {
	Post(ctx context.Context, path string, body any) (map[string]any, error)
}

// Input is what the prefill is built from: the declaration's registered
// reference (office/year/serial/number), the codes it was declared under, and
// the container rows to fall back to — one per dispatch note the consignment
// registered.
type Input struct {
	CusdecRef     string
	DeclarantCode string
	CompanyCode   string
	Fallback      []map[string]any
}

// Result is the prefilled boat note form, and whether it was built from the
// retrieved declaration. Form always carries a containers array.
type Result struct {
	Form      map[string]any
	Retrieved bool
}

// Prefill retrieves the declaration and the dispatch notes registered against
// it, and maps them onto the boat note form: one container row per dispatch note
// that is not cancelled, so the containers on the boat note match the ones
// Customs holds.
//
// Nothing here fails the step. A declaration that cannot be retrieved leaves
// the trader the fallback rows and the form to fill, as before; a dispatch note
// that cannot be retrieved keeps its row with only its number.
func Prefill(ctx context.Context, call Caller, in Input) Result {
	fallback := Result{Form: map[string]any{"containers": rowsOrEmpty(in.Fallback)}}

	ref, err := cdn.ParseDocumentReference(in.CusdecRef)
	if err != nil {
		slog.WarnContext(ctx, "boat note prefill: declaration reference unreadable; using the consignment's dispatch notes",
			"cusdec_ref", in.CusdecRef, "error", err)
		return fallback
	}
	resp, err := call.Post(ctx, DeclarationRetrievePath, map[string]any{
		"cusDecRef":     ref,
		"declarantCode": in.DeclarantCode,
		"companyCode":   in.CompanyCode,
	})
	if err != nil {
		slog.WarnContext(ctx, "boat note prefill: declaration not retrieved; using the consignment's dispatch notes",
			"cusdec_ref", in.CusdecRef, "error", err)
		return fallback
	}
	decl := firstObject(resp, "cusdec", "cusDec")
	if decl == nil {
		slog.WarnContext(ctx, "boat note prefill: retrieve answer carries no declaration; using the consignment's dispatch notes",
			"cusdec_ref", in.CusdecRef)
		return fallback
	}

	form := declarationFields(decl)

	refs := activeCDNRefs(resp)
	if len(refs) == 0 {
		slog.WarnContext(ctx, "boat note prefill: declaration lists no active dispatch notes; using the consignment's",
			"cusdec_ref", in.CusdecRef)
		form["containers"] = rowsOrEmpty(in.Fallback)
		return Result{Form: form, Retrieved: true}
	}

	notes := make([]map[string]any, 0, len(refs))
	rows := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		number := fmt.Sprintf("%s/%s/%s/%d", r.Office, r.Year, r.Serial, r.Number)
		note := retrieveCDN(ctx, call, r, in)
		notes = append(notes, note)
		rows = append(rows, containerRow(number, note))
	}
	form["containers"] = rows
	dispatchFields(form, notes)
	return Result{Form: form, Retrieved: true}
}

// retrieveCDN fetches one dispatch note (§7.8). The company code must match the
// note's shipper or consignee; the exporter's code is the shipper's. A note that
// cannot be retrieved is returned empty.
func retrieveCDN(ctx context.Context, call Caller, ref cdn.DocumentReference, in Input) map[string]any {
	resp, err := call.Post(ctx, CDNRetrievePath, map[string]any{
		"cdnRef":        ref,
		"declarantCode": in.DeclarantCode,
		"companyCode":   in.CompanyCode,
	})
	if err != nil {
		slog.WarnContext(ctx, "boat note prefill: dispatch note not retrieved; its row keeps only its number",
			"cdn_ref", ref, "error", err)
		return nil
	}
	return firstObject(resp, "cdn")
}

// declarationFields maps the declaration (Annex A) onto the boat note.
func declarationFields(decl map[string]any) map[string]any {
	base := nested(decl, "baseGeneralSegment")
	general := nested(decl, "generalSegment")
	exporter := nested(base, "exporter")
	importer := nested(base, "importer")

	form := map[string]any{}
	setString(form, "shipper", party(exporter))
	setString(form, "consignee", party(importer))
	setString(form, "exporters_registration_no", str(exporter, "id"))
	setString(form, "vessel", str(general, "transportVesselName"))
	setString(form, "voyage_no_date", str(general, "transportVoyageName"))
	setString(form, "port_of_discharge", str(general, "placeOfDischarge"))
	if n, ok := num(general["numberOfPackages"]); ok {
		form["number_of_packages"] = formatNumber(n)
	}

	var descriptions, classifications []string
	var gross, net float64
	var hasGross, hasNet bool
	for _, item := range objects(decl["goodsShipments"]) {
		commodity := nested(item, "commodity")
		descriptions = appendUnique(descriptions, str(commodity, "commercialDescription"))
		classifications = appendUnique(classifications, str(commodity, "classification"))
		measure := nested(commodity, "goodsMeasure")
		if v, ok := num(nested(measure, "grossMassMeasure")["value"]); ok {
			gross, hasGross = gross+v, true
		}
		if v, ok := num(nested(measure, "netWeightMeasure")["value"]); ok {
			net, hasNet = net+v, true
		}
	}
	setString(form, "description_of_goods", strings.Join(descriptions, "; "))
	setString(form, "cccn_no", strings.Join(classifications, "; "))
	if hasGross {
		form["gross_mass"] = gross
	}
	if hasNet {
		form["net_mass"] = net
	}
	return form
}

// dispatchFields fills, from the dispatch notes (Annex B), what the declaration
// does not carry: the port of loading, the operators, the container marks and
// the volume. The voyage number and date a note was booked on replace the
// declaration's voyage name, which carries no date; other fields the
// declaration already filled are kept.
func dispatchFields(form map[string]any, notes []map[string]any) {
	var marks []string
	var cube float64
	var hasCube, voyageFromNote bool
	for _, n := range notes {
		if n == nil {
			continue
		}
		if voyage := strings.Join(nonEmpty(str(n, "voyageNumber"), str(n, "voyageDateAsString")), " / "); voyage != "" && !voyageFromNote {
			form["voyage_no_date"], voyageFromNote = voyage, true
		}
		setIfAbsent(form, "vessel", str(n, "vessel"))
		setIfAbsent(form, "port_of_loading", str(n, "placeOfLoading"))
		setIfAbsent(form, "port_of_discharge", str(n, "placeOfDischarge"))
		setIfAbsent(form, "vessel_op_code", str(n, "vesselOpCode"))
		setIfAbsent(form, "cont_op_code", str(n, "contOpCode"))
		marks = appendUnique(marks, firstNonEmpty(str(n, "containerMark"), str(n, "containerNumber")))
		if v, ok := num(n["volume"]); ok {
			cube, hasCube = cube+v, true
		}
	}
	setString(form, "marks_nos_container_no", strings.Join(marks, ", "))
	if hasCube {
		form["cube_m3"] = cube
	}
}

// containerRow is one boat note container row for a dispatch note.
func containerRow(number string, note map[string]any) map[string]any {
	row := map[string]any{"cdn_number": number}
	setString(row, "container_number", str(note, "containerNumber"))
	setString(row, "customs_seal_no", str(note, "sealNo"))
	return row
}

// activeCDNRefs lists the dispatch notes the retrieve answer registers against
// the declaration (cdnRefs), leaving out any Customs cancelled.
func activeCDNRefs(resp map[string]any) []cdn.DocumentReference {
	raw, ok := nested(resp, "payload")["cdnRefs"]
	if !ok {
		raw = resp["cdnRefs"]
	}
	var refs []cdn.DocumentReference
	for _, r := range objects(raw) {
		if strings.Contains(strings.ToLower(str(r, "status")), "cancel") {
			continue
		}
		n, ok := num(r["number"])
		if !ok {
			continue
		}
		ref := cdn.DocumentReference{Office: str(r, "office"), Year: str(r, "year"), Serial: str(r, "serial"), Number: int(n)}
		if ref.IsValid() {
			refs = append(refs, ref)
		}
	}
	return refs
}

// firstObject finds the named object in a retrieve answer, which the
// specification shows under payload but not consistently (§6.7, §7.8).
func firstObject(resp map[string]any, keys ...string) map[string]any {
	for _, holder := range []map[string]any{nested(resp, "payload"), resp} {
		for _, key := range keys {
			if obj, ok := holder[key].(map[string]any); ok && len(obj) > 0 {
				return obj
			}
		}
	}
	return nil
}

func party(p map[string]any) string {
	return strings.Join(nonEmpty(str(p, "name"), str(p, "address")), ", ")
}

func rowsOrEmpty(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
}

func nested(m map[string]any, key string) map[string]any {
	v, _ := m[key].(map[string]any)
	return v
}

func str(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return formatNumber(v)
	default:
		return ""
	}
}

func num(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func formatNumber(f float64) string {
	if f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func objects(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func setString(m map[string]any, key, value string) {
	if value != "" {
		m[key] = value
	}
}

func setIfAbsent(m map[string]any, key, value string) {
	if _, ok := m[key]; !ok {
		setString(m, key, value)
	}
}

func appendUnique(list []string, value string) []string {
	if value == "" {
		return list
	}
	for _, v := range list {
		if v == value {
			return list
		}
	}
	return append(list, value)
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
