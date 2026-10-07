package cusdec

import (
	"fmt"
	"strings"
	"time"

	"github.com/OpenNSW/nsw-srilanka/external-integration/customs/asycuda/nswid"
)

// The SLC Edge CusDec submission payload, per Annex A of the ASYCUDA ↔ NSW
// Interface Specification v1.5.
//
// The trader form (customs-cusdec--user-form in one-trade-artifacts) is shaped
// around the ASYCUDA World data-entry screens — identification / traders /
// tarification — while Annex A is shaped around the SAD segments. BuildPayload
// below is the translation between the two; nothing else in the flow should
// need to know either shape.
//
// One place where Annex A and the working sample payload disagree, resolved in
// favour of the sample because it is what the endpoint actually accepts:
//
//   - Annex A names the remittance block "Remittance. 1" (singular); the
//     accepted payload sends "remittances" as an array.
type Submission struct {
	Properties          Properties   `json:"properties"`
	BaseGeneralSegment  BaseSegment  `json:"baseGeneralSegment"`
	GeneralSegment      GeneralSeg   `json:"generalSegment"`
	GoodsShipments      []GoodsItem  `json:"goodsShipments"`
	Remittances         []Remittance `json:"remittances"`
	SupportingDocuments []SupportDoc `json:"supportingDocuments,omitempty"`
}

// Properties is Annex A's properties block: who is submitting, and the
// identifier that tells one logical submission from another (§2.2). See
// nswid.For for how that identifier is derived and why.
type Properties struct {
	Submitter string `json:"submitter"`
	NswID     string `json:"nswId"`
}

// Amount is Annex A §4.3 AmountType. CurrencyID is omitted when empty: the
// spec makes it mandatory only where a value is present, and unused cost
// lines are sent as {"value":0}.
type Amount struct {
	Value      float64 `json:"value"`
	CurrencyID string  `json:"currencyID,omitempty"`
}

// Measure is Annex A §4.4 MeasureType.
type Measure struct {
	Value    float64 `json:"value"`
	UnitCode string  `json:"unitCode,omitempty"`
}

// Valuation is §4.2 CustomsValuation, the customs-value breakdown carried by
// both the general segment (totalCustomsValuation) and each item
// (customsValue). Every element is an Amount (§4.3).
//
// Spec v1.6 typed both fields as a bare AmountType and this package followed
// the accepted payload instead; v1.7 adds §4.2 and the two now agree.
type Valuation struct {
	ChargeAmount    Amount `json:"chargeAmount"`
	ExternalFreight Amount `json:"externalFreight"`
	InternalFreight Amount `json:"internalFreight"`
	Insurance       Amount `json:"insurance"`
	OtherCost       Amount `json:"otherCost"`
	Deductions      Amount `json:"deductions"`
}

type Party struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	CountryCode string `json:"countryCode"`
}

type BaseSegment struct {
	DeclarationType      string `json:"declarationType"`
	DeclarationProcedure string `json:"declarationProcedure"`
	DeclarationMode      string `json:"declarationMode"`
	OfficeCode           string `json:"officeCode"`
	ManifestRegNumber    string `json:"manifestRegNumber"`
	Importer             Party  `json:"importer"`
	Exporter             Party  `json:"exporter"`
	DeclarantCode        string `json:"declarantCode"`
}

type GeneralSeg struct {
	NumberOfItems                  int       `json:"numberOfItems"`
	NumberOfPackages               int       `json:"numberOfPackages"`
	CountryOfExportCode            string    `json:"countryOfExportCode"`
	CountryOfDestination           string    `json:"countryOfDestination"`
	CountryFirstDestination        string    `json:"countryFirstDestination"`
	TransportVesselName            string    `json:"transportVesselName"`
	TransportVesselNameNationality string    `json:"transportVesselNameNationality"`
	TransportVoyageName            string    `json:"transportVoyageName"`
	TransportVoyageNameNationality string    `json:"transportVoyageNameNationality"`
	DeliveryTerms                  string    `json:"deliveryTerms"`
	DeliveryTermsPlace             string    `json:"deliveryTermsPlace"`
	ModeOfTransportAtBorder        string    `json:"modeOfTransportAtBorder"`
	PlaceOfDischarge               string    `json:"placeOfDischarge"`
	BorderOffice                   string    `json:"borderOffice"`
	TotalCustomsValuation          Valuation `json:"totalCustomsValuation"`
	ContainerFlag                  bool      `json:"containerFlag"`
	NumberOfContainers             int       `json:"numberOfContainers"`
	WarehouseCode                  string    `json:"warehouseCode"`
	WarehouseDelay                 int       `json:"warehouseDelay"`
	DeferredPayment                string    `json:"deferredPayment"`
}

type Commodity struct {
	CommercialDescription  string       `json:"commercialDescription"`
	CommercialDescription1 string       `json:"commercialDescription1"`
	CommodityDescription   string       `json:"commodityDescription"`
	Classification         string       `json:"classification"`
	GoodsMeasure           GoodsMeasure `json:"goodsMeasure"`
}

type GoodsMeasure struct {
	GrossMassMeasure Measure `json:"grossMassMeasure"`
	NetWeightMeasure Measure `json:"netWeightMeasure"`
	TariffQuantity   Measure `json:"tariffQuantity"`
}

type GovernmentProcedure struct {
	ExtendedProcedure string `json:"extendedProcedure"`
	NationalProcedure string `json:"nationalProcedure"`
}

type GoodsItem struct {
	SequenceNumeric     int                 `json:"sequenceNumeric"`
	CustomsValue        Valuation           `json:"customsValue"`
	Commodity           Commodity           `json:"commodity"`
	GovernmentProcedure GovernmentProcedure `json:"governmentProcedure"`
	CountryOfOriginCode string              `json:"countryOfOriginCode"`
	GoodsPreference     string              `json:"goodsPreference"`
	ItemPackage         Measure             `json:"itemPackage"`
	Bol                 string              `json:"bol"`
	BolSplit            string              `json:"bolSplit"`
	MarksAndNumbers     string              `json:"marksAndNumbers"`
}

type Remittance struct {
	BankCode        string `json:"bankCode"`
	Reference       string `json:"reference"`
	TermsOfPayment  string `json:"termsOfPayment"`
	RemittanceValue Amount `json:"remittanceValue"`
}

// SupportDoc is one entry of the supportingDocuments array.
//
// The trader form groups supporting documents into two kinds, and the sequence
// number the endpoint expects differs between them:
//
//   - a scanned document, which carries documentCode, fileBase64 and fileName,
//     has a matching fileN part in the multipart request, and always travels
//     with sequenceNumber 0; and
//   - a metadata document, which references a document held elsewhere, needs
//     itemSequence, documentCode, documentId and dateAsString, attaches no
//     file, and is numbered from 1 upward.
//
// FileBase64 is the storage key passed through from the form unchanged (despite
// the name it is not encoded bytes; the bytes travel as the fileN part).
// FileName must match the filename on the corresponding fileN part (§6.1.2),
// which BuildPayload guarantees by deriving both from the same storage key.
type SupportDoc struct {
	SequenceNumber int    `json:"sequenceNumber"`
	DocumentCode   string `json:"documentCode"`
	FileBase64     string `json:"fileBase64,omitempty"`
	FileName       string `json:"fileName,omitempty"`

	// The metadata half. ItemSequence is the item the document applies to and
	// must match a goodsShipments[].sequenceNumeric in the same declaration; it
	// is forwarded as the string the form sends (e.g. "003"). DateAsString is
	// dd/MM/yyyy, which is not the ISO-8601 the rest of the interface uses.
	ItemSequence string `json:"itemSequence,omitempty"`
	DocumentID   string `json:"documentId,omitempty"`
	DateAsString string `json:"dateAsString,omitempty"`

	// storageKey is where the bytes live; it never reaches the wire. Empty for
	// a metadata document, which is what tells the two apart when the caller
	// collects the files to attach.
	storageKey string
}

// HasFile reports whether this entry expects a fileN part in the multipart
// request. A metadata document does not.
func (d SupportDoc) HasFile() bool { return d.storageKey != "" }

// Constants the form does not collect because they never vary for this flow.
const (
	// submitterChannel identifies NSW as the submitting channel.
	submitterChannel = "1"
	// declarationModeElectronic is fixed at "E"; Annex A rejects anything else.
	declarationModeElectronic = "E"
)

// BuildPayload translates a trader form submission into the Annex A payload and
// the list of documents to attach. The returned SupportDocs carry the storage
// key each file must be fetched from, in the same order as the payload's
// supportingDocuments array, so fileN and supportingDocuments[N-1] line up.
//
// Missing optional values become their zero value rather than an error: the
// endpoint validates the document on integration and reports field-level
// problems through the errors object, which produces a far better trader
// message than a local guess at what Customs will accept.
func BuildPayload(form map[string]any, previousEdgeID string) (Submission, []SupportDoc, error) {
	if len(form) == 0 {
		return Submission{}, nil, fmt.Errorf("customs: empty declaration form")
	}

	ident := nested(form, "identification")
	traders := nested(form, "traders")
	general := nested(form, "generalInfo")
	transport := nested(form, "transport")
	financial := nested(form, "financial")
	valuation := nested(form, "valuation")
	packages := nested(form, "packages")

	exporter := nested(traders, "exporter")
	consignee := nested(traders, "consignee")
	declarant := nested(traders, "declarant")

	items, err := buildItems(form)
	if err != nil {
		return Submission{}, nil, err
	}

	docs, err := buildSupportDocs(form)
	if err != nil {
		return Submission{}, nil, err
	}

	sub := Submission{
		Properties: Properties{Submitter: submitterChannel},
		BaseGeneralSegment: BaseSegment{
			DeclarationType:      str(ident, "declarationType"),
			DeclarationProcedure: str(ident, "generalProcedureCode"),
			DeclarationMode:      declarationModeElectronic,
			OfficeCode:           str(ident, "officeCode"),
			ManifestRegNumber:    str(ident, "manifestRegNumber"),
			// The form collects the consignee, which is the importer in an
			// export declaration.
			Importer: Party{
				ID:          str(consignee, "code"),
				Name:        str(consignee, "name"),
				Address:     str(consignee, "address"),
				CountryCode: str(consignee, "countryCode"),
			},
			Exporter: Party{
				ID:          str(exporter, "code"),
				Name:        str(exporter, "name"),
				Address:     str(exporter, "address"),
				CountryCode: str(exporter, "countryCode"),
			},
			DeclarantCode: str(declarant, "code"),
		},
		GeneralSegment: GeneralSeg{
			// Annex A defines this as the count of goodsShipments[], so it is
			// derived rather than collected — the two cannot disagree.
			NumberOfItems:                  len(items),
			NumberOfPackages:               integer(packages, "totalPackages"),
			CountryOfExportCode:            str(general, "exportCountryCode"),
			CountryOfDestination:           str(general, "destinationCountryCode"),
			CountryFirstDestination:        str(general, "countryOfFirstDestination"),
			TransportVesselName:            str(transport, "vesselName"),
			TransportVesselNameNationality: str(transport, "transportNationality"),
			TransportVoyageName:            str(transport, "voyageNo"),
			TransportVoyageNameNationality: str(transport, "voyageNationality"),
			DeliveryTerms:                  str(transport, "deliveryTermsCode"),
			DeliveryTermsPlace:             str(transport, "deliveryTermsPlace"),
			ModeOfTransportAtBorder:        str(transport, "modeOfTransport"),
			PlaceOfDischarge:               str(transport, "placeOfDischargeCode"),
			BorderOffice:                   str(transport, "borderOfficeCode"),
			TotalCustomsValuation:          buildDeclarationValuation(valuation),
			ContainerFlag:                  boolean(transport, "containerized"),
			NumberOfContainers:             integer(form, "containerCount"),
			WarehouseCode:                  str(transport, "warehouseCode"),
			WarehouseDelay:                 integer(transport, "warehouseDelay"),
			DeferredPayment:                str(financial, "deferredPayment"),
		},
		GoodsShipments:      items,
		Remittances:         buildRemittances(financial, str(nested(valuation, "invoiceAmount"), "currencyCode")),
		SupportingDocuments: docs,
	}

	// Derived last, from the submission as it will be sent: the field is empty
	// while the digest is taken, so the identifier does not depend on itself.
	sub.Properties.NswID = nswid.For(sub, previousEdgeID)

	return sub, docs, nil
}

// buildDeclarationValuation maps the header valuation onto
// generalSegment.totalCustomsValuation. Each cost is amount + currencyCode.
func buildDeclarationValuation(v map[string]any) Valuation {
	section := func(key string) Amount {
		s := nested(v, key)
		val := number(s, "amount")
		if val == 0 {
			return Amount{Value: 0}
		}
		return Amount{Value: val, CurrencyID: str(s, "currencyCode")}
	}
	invoice := nested(v, "invoiceAmount")
	return Valuation{
		ChargeAmount:    Amount{Value: number(invoice, "amount"), CurrencyID: str(invoice, "currencyCode")},
		ExternalFreight: section("externalFreight"),
		InternalFreight: section("internalFreight"),
		Insurance:       section("insurance"),
		OtherCost:       section("otherCosts"),
		Deductions:      section("deductions"),
	}
}

// buildItemValuation maps an item line onto goodsShipments[].customsValue.
// Annex A still requires the six-part block; the form only collects
// invoiceAmount on the line, so chargeAmount is filled and the other five
// costs travel as {"value":0}.
func buildItemValuation(v map[string]any) Valuation {
	invoice := nested(v, "invoiceAmount")
	return Valuation{
		ChargeAmount: Amount{Value: number(invoice, "amount"), CurrencyID: str(invoice, "currencyCode")},
	}
}

func buildItems(form map[string]any) ([]GoodsItem, error) {
	raw, ok := form["items"].([]any)
	if !ok || len(raw) == 0 {
		return nil, fmt.Errorf("customs: declaration has no items")
	}

	items := make([]GoodsItem, 0, len(raw))
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("customs: item %d is not an object", i+1)
		}

		tarif := nested(m, "tarification")
		goods := nested(m, "goodsDescription")
		val := nested(m, "valuation")
		pkg := nested(m, "packages")
		supp := nested(tarif, "supplementaryUnit")

		itemValuation := buildItemValuation(val)

		items = append(items, GoodsItem{
			// Annex A requires a unique item number; position in the array is
			// the only ordering the form has, and it is what numberOfItems and
			// the errors object's segment keys are counted against.
			SequenceNumeric: i + 1,
			CustomsValue:    itemValuation,
			Commodity: Commodity{
				CommercialDescription:  str(goods, "commercialDescription"),
				CommercialDescription1: str(goods, "commercialDescription1"),
				CommodityDescription:   str(goods, "description"),
				Classification:         str(tarif, "hsCode"),
				GoodsMeasure: GoodsMeasure{
					GrossMassMeasure: Measure{Value: number(val, "grossWeight"), UnitCode: massUnit},
					NetWeightMeasure: Measure{Value: number(val, "netWeight"), UnitCode: massUnit},
					TariffQuantity: Measure{
						Value:    number(supp, "quantity"),
						UnitCode: str(supp, "code"),
					},
				},
			},
			GovernmentProcedure: GovernmentProcedure{
				ExtendedProcedure: str(tarif, "extendedProcedureCode"),
				NationalProcedure: str(tarif, "nationalProcedureCode"),
			},
			CountryOfOriginCode: str(goods, "originCountryCode"),
			GoodsPreference:     str(tarif, "preferenceCode"),
			ItemPackage: Measure{
				Value:    float64(integer(pkg, "quantity")),
				UnitCode: str(pkg, "kindCode"),
			},
			Bol:             str(m, "bol"),
			BolSplit:        str(m, "bolSplit"),
			MarksAndNumbers: str(m, "marksAndNumbers"),
		})
	}
	return items, nil
}

// massUnit is the unit the form's weight fields are captured in; the form
// labels them in kilograms and offers no unit selector.
const massUnit = "KG"

func buildRemittances(financial map[string]any, currency string) []Remittance {
	amount := number(financial, "remittanceAmount")
	r := Remittance{
		BankCode:       str(financial, "bankCode"),
		Reference:      str(financial, "bankReference"),
		TermsOfPayment: str(financial, "paymentTermsCode"),
	}
	// AmountType pairs the value with its currency (§4.3). The form declares
	// the invoice currency on valuation.invoiceAmount.
	if amount != 0 {
		r.RemittanceValue = Amount{Value: amount, CurrencyID: currency}
	}
	// Annex A marks the whole block mandatory, but an empty one carries no
	// information and the endpoint rejects it more clearly than a block of
	// zero values would.
	if r.BankCode == "" && r.Reference == "" && r.TermsOfPayment == "" && r.RemittanceValue.Value == 0 {
		return nil
	}
	return []Remittance{r}
}

// buildSupportDocs maps the form's supporting-document group onto Annex A
// entries. The form sends supportingDocuments as an object with two arrays:
//
//   - scannedDocuments, each an uploaded PDF, which always travel with
//     sequenceNumber 0; and
//   - metaDocuments, each a reference to a document held elsewhere, numbered
//     from 1 upward.
//
// Scanned entries come first so the array order and the fileN part numbering
// stay in step. fileName (and fileBase64) are the storage key rather than the
// trader's original filename: §6.1.2 matches part filenames against
// supportingDocuments entries one-to-one, and two files uploaded under the same
// original name would make that match ambiguous. Keys are unique by
// construction and keep the uploaded extension, so they satisfy both that rule
// and §8's PDF requirement.
func buildSupportDocs(form map[string]any) ([]SupportDoc, error) {
	group := nested(form, "supportingDocuments")
	if len(group) == 0 {
		return nil, nil
	}

	scanned, _ := group["scannedDocuments"].([]any)
	meta, _ := group["metaDocuments"].([]any)

	docs := make([]SupportDoc, 0, len(scanned)+len(meta))

	// Scanned documents: sequenceNumber is always 0 (§8).
	for i, entry := range scanned {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("customs: scanned document %d is not an object", i+1)
		}

		key := strings.TrimSpace(str(m, "fileBase64"))
		code := str(m, "documentCode")
		if key == "" || code == "" {
			return nil, fmt.Errorf(
				"customs: scanned document %d is missing its file or document code", i+1)
		}

		docs = append(docs, SupportDoc{
			SequenceNumber: 0,
			DocumentCode:   code,
			FileBase64:     key,
			FileName:       key,
			storageKey:     key,
		})
	}

	// Metadata documents: sequenceNumber runs from 1 upward.
	for i, entry := range meta {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("customs: meta document %d is not an object", i+1)
		}

		doc := SupportDoc{
			SequenceNumber: i + 1,
			DocumentCode:   str(m, "documentCode"),
			ItemSequence:   str(m, "itemSequence"),
			DocumentID:     strings.TrimSpace(str(m, "documentId")),
			DateAsString:   formatDMY(str(m, "dateAsString")),
		}

		// A metadata document stands in for bytes held elsewhere, so it must
		// carry the reference fields that identify the document; without them
		// the endpoint rejects it (400).
		if doc.DocumentCode == "" || doc.ItemSequence == "" || doc.DocumentID == "" || doc.DateAsString == "" {
			return nil, fmt.Errorf(
				"customs: meta document %d is missing the item number, document code, reference or date it needs", i+1)
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

// formatDMY converts the form's ISO date (yyyy-MM-dd) to the dd/MM/yyyy Annex A
// expects for a supporting document. An unexpected shape is passed through
// trimmed rather than dropped, so a value the endpoint can still interpret is
// not silently lost.
func formatDMY(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", v)
	if err != nil {
		return v
	}
	return t.Format("02/01/2006")
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
	default:
		return 0
	}
}

func integer(m map[string]any, key string) int {
	return int(number(m, key))
}

func boolean(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}
