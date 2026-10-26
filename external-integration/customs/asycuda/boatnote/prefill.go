// Package boatnote prefills the boat note application from what Sri Lanka
// Customs holds for the consignment: the declaration as it stands in ASYCUDA
// World, and the Cargo Dispatch Notes registered against it.
package boatnote

import (
	"context"
	"log/slog"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/document"
)

// CDNRetrievePath is SLC Edge's dispatch note retrieve endpoint (§7.8).
const CDNRetrievePath = "/api/cdn/v1/retrieve"

// Caller posts a JSON body to an SLC Edge path and returns the decoded answer.
type Caller interface {
	Post(ctx context.Context, path string, body any) (map[string]any, error)
}

// Input is what the prefill is built from: the declaration as the declaration
// retrieve (§6.7) captured it, with the dispatch notes listed against it
// (cdnRefs); the codes it was declared under, which the dispatch note retrieve
// matches; and the container rows to fall back to, one per dispatch note the
// consignment registered.
type Input struct {
	Declaration   map[string]any
	CDNRefs       []map[string]any
	DeclarantCode string
	CompanyCode   string
	Fallback      []map[string]any
}

// Result is the prefilled boat note form, and whether it was built from what
// Customs holds: the retrieved declaration and, when it lists dispatch notes, at
// least one of them. Form always carries a containers array.
type Result struct {
	Form      map[string]any
	Retrieved bool
}

// Prefill maps the retrieved declaration, and each dispatch note listed against
// it that is not cancelled, onto the boat note form: one container row per
// note, so the containers on the boat note match the ones Customs holds. It
// retrieves the dispatch notes (§7.8); the declaration comes already retrieved.
//
// Nothing here fails the step. Without a declaration the trader gets the
// fallback rows and the form to fill, as before; a dispatch note that cannot be
// retrieved keeps its row with only its number.
func Prefill(ctx context.Context, call Caller, in Input) Result {
	if in.Declaration == nil {
		slog.WarnContext(ctx, "boat note prefill: no retrieved declaration; using the consignment's dispatch notes")
		return Result{Form: map[string]any{"containers": rowsOrEmpty(in.Fallback)}}
	}

	form := declarationFields(in.Declaration)

	refs := activeCDNRefs(in.CDNRefs)
	if len(refs) == 0 {
		slog.WarnContext(ctx, "boat note prefill: declaration lists no active dispatch notes; using the consignment's")
		form["containers"] = rowsOrEmpty(in.Fallback)
		return Result{Form: form, Retrieved: true}
	}

	notes := make([]map[string]any, 0, len(refs))
	rows := make([]map[string]any, 0, len(refs))
	retrieved := false
	for _, r := range refs {
		note := retrieveCDN(ctx, call, r, in)
		retrieved = retrieved || note != nil
		notes = append(notes, note)
		rows = append(rows, containerRow(r.String(), note))
	}
	form["containers"] = rows
	dispatchFields(form, notes)
	return Result{Form: form, Retrieved: retrieved}
}

// retrieveCDN fetches one dispatch note (§7.8). The company code must match the
// note's shipper or consignee; the exporter's code is the shipper's. A note that
// cannot be retrieved is returned empty.
func retrieveCDN(ctx context.Context, call Caller, ref document.Reference, in Input) map[string]any {
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
	return document.FirstObject(resp, "cdn")
}

// declarationFields maps the declaration (Annex A) onto the boat note.
func declarationFields(decl map[string]any) map[string]any {
	base := document.Nested(decl, "baseGeneralSegment")
	general := document.Nested(decl, "generalSegment")
	exporter := document.Nested(base, "exporter")
	importer := document.Nested(base, "importer")

	form := map[string]any{}
	setString(form, "shipper", party(exporter))
	setString(form, "consignee", party(importer))
	setString(form, "exporters_registration_no", document.Text(exporter, "id"))
	setString(form, "vessel", document.Text(general, "transportVesselName"))
	setString(form, "voyage_no_date", document.Text(general, "transportVoyageName"))
	setString(form, "port_of_discharge", document.Text(general, "placeOfDischarge"))
	if n, ok := document.Float(general["numberOfPackages"]); ok {
		form["number_of_packages"] = document.FormatNumber(n)
	}

	var descriptions, classifications []string
	var gross, net float64
	var hasGross, hasNet bool
	for _, item := range document.Objects(decl["goodsShipments"]) {
		commodity := document.Nested(item, "commodity")
		descriptions = appendUnique(descriptions, document.Text(commodity, "commercialDescription"))
		classifications = appendUnique(classifications, document.Text(commodity, "classification"))
		measure := document.Nested(commodity, "goodsMeasure")
		if v, ok := document.Float(document.Nested(measure, "grossMassMeasure")["value"]); ok {
			gross, hasGross = gross+v, true
		}
		if v, ok := document.Float(document.Nested(measure, "netWeightMeasure")["value"]); ok {
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
		if voyage := strings.Join(nonEmpty(document.Text(n, "voyageNumber"), document.Text(n, "voyageDateAsString")), " / "); voyage != "" && !voyageFromNote {
			form["voyage_no_date"], voyageFromNote = voyage, true
		}
		setIfAbsent(form, "vessel", document.Text(n, "vessel"))
		setIfAbsent(form, "port_of_loading", document.Text(n, "placeOfLoading"))
		setIfAbsent(form, "port_of_discharge", document.Text(n, "placeOfDischarge"))
		setIfAbsent(form, "vessel_op_code", document.Text(n, "vesselOpCode"))
		setIfAbsent(form, "cont_op_code", document.Text(n, "contOpCode"))
		marks = appendUnique(marks, firstNonEmpty(document.Text(n, "containerMark"), document.Text(n, "containerNumber")))
		if v, ok := document.Float(n["volume"]); ok {
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
	setString(row, "container_number", document.Text(note, "containerNumber"))
	setString(row, "customs_seal_no", document.Text(note, "sealNo"))
	return row
}

// cancelledStatus is ASYCUDA World's status for a cancelled dispatch note
// (§7.7), the only one listed against a declaration that is not live: a note
// whose integration failed is never registered, so it is never listed (§7.2).
const cancelledStatus = "Cancelled"

// activeCDNRefs reads the dispatch notes listed against the declaration
// (cdnRefs), leaving out any Customs cancelled.
func activeCDNRefs(cdnRefs []map[string]any) []document.Reference {
	var refs []document.Reference
	for _, r := range cdnRefs {
		if strings.EqualFold(document.Str(r, "status"), cancelledStatus) {
			continue
		}
		n, err := document.WholeNumber(r["number"])
		if err != nil {
			continue
		}
		ref := document.Reference{Office: document.Text(r, "office"), Year: document.Text(r, "year"), Serial: document.Text(r, "serial"), Number: n}
		if ref.IsValid() {
			refs = append(refs, ref)
		}
	}
	return refs
}

func party(p map[string]any) string {
	return strings.Join(nonEmpty(document.Text(p, "name"), document.Text(p, "address")), ", ")
}

func rowsOrEmpty(rows []map[string]any) []map[string]any {
	if rows == nil {
		return []map[string]any{}
	}
	return rows
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
