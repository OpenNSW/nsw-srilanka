package cusdec

import (
	"fmt"
	"strings"

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
// Two places where Annex A and the working sample payload disagree, resolved
// in favour of the sample because it is what the endpoint actually accepts:
//
//   - Annex A types totalCustomsValuation and goodsShipments[].customsValue as
//     a single AmountType. The accepted payload sends a six-part valuation
//     object (chargeAmount, externalFreight, internalFreight, insurance,
//     otherCost, deductions), which is what Valuation models below.
//   - Annex A names the remittance block "Remittance. 1" (singular); the
//     accepted payload sends "remittances" as an array.
type Submission struct {
	Properties          Properties   `json:"properties"`
	Submitter           string       `json:"submitter"`
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

// Amount is Annex A §4.2 AmountType. CurrencyID is omitted when empty: the
// spec makes it mandatory only where a value is present, and the accepted
// payload sends bare {"value":0} for the unused cost lines.
type Amount struct {
	Value      float64 `json:"value"`
	CurrencyID string  `json:"currencyID,omitempty"`
}

// Measure is Annex A §4.3 MeasureType.
type Measure struct {
	Value    float64 `json:"value"`
	UnitCode string  `json:"unitCode,omitempty"`
}

// Valuation is the customs-value breakdown carried by both the general segment
// (totalCustomsValuation) and each item (customsValue).
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
	TransportVoageName             string    `json:"transportVoageName"`
	TransportVoageNameNationality  string    `json:"transportVoageNameNationality"`
	DeliveryTerms                  string    `json:"deliveryTerms"`
	DeliveryTermsPlace             string    `json:"deliveryTermsPlace"`
	ModeOfTransportAtBorder        string    `json:"modeOfTransportAtBorder"`
	PlaceOfDischarge               string    `json:"placeOfDischarge"`
	BorderOffice                   string    `json:"borderOffice"`
	TotalCustomsValuation          Valuation `json:"totalCustomsValuation"`
	IsContainer                    bool      `json:"isContainer"`
	NumberOfContainers             int       `json:"numberOfContainers"`
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
}

type Remittance struct {
	BankCode       string  `json:"bankCode"`
	Reference      string  `json:"reference"`
	TermsOfPayment string  `json:"termsOfPayment"`
	Amount         float64 `json:"amount"`
}

// SupportDoc is one entry of the supportingDocuments array. FileName must match
// the filename on the corresponding fileN multipart part (§6.1.2), which
// BuildPayload guarantees by deriving both from the same storage key.
type SupportDoc struct {
	FileName       string `json:"fileName"`
	DocumentCode   string `json:"documentCode"`
	SequenceNumber int    `json:"sequenceNumber"`

	// storageKey is where the bytes live; it never reaches the wire.
	storageKey string
}

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
		Submitter:  submitterChannel,
		BaseGeneralSegment: BaseSegment{
			DeclarationType:      str(ident, "declarationType"),
			DeclarationProcedure: str(ident, "generalProcedureCode"),
			DeclarationMode:      declarationModeElectronic,
			OfficeCode:           str(ident, "officeCode"),
			ManifestRegNumber:    str(ident, "manifestRegNumber"),
			// The form collects the consignee, which is the importer in an
			// export declaration; it has no field for their trader code.
			Importer: Party{
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
			TransportVoageName:             str(transport, "voyageNo"),
			TransportVoageNameNationality:  str(transport, "voyageNationality"),
			DeliveryTerms:                  str(transport, "deliveryTermsCode"),
			DeliveryTermsPlace:             str(transport, "deliveryTermsPlace"),
			ModeOfTransportAtBorder:        str(transport, "modeOfTransport"),
			PlaceOfDischarge:               str(transport, "placeOfDischargeCode"),
			BorderOffice:                   str(transport, "borderOfficeCode"),
			TotalCustomsValuation:          buildValuation(valuation),
			IsContainer:                    boolean(transport, "containerized"),
			NumberOfContainers:             integer(form, "containerCount"),
		},
		GoodsShipments:      items,
		Remittances:         buildRemittances(financial),
		SupportingDocuments: docs,
	}

	// Derived last, from the submission as it will be sent: the field is empty
	// while the digest is taken, so the identifier does not depend on itself.
	sub.Properties.NswID = nswid.For(sub, previousEdgeID)

	return sub, docs, nil
}

// buildValuation maps the form's foreign-currency valuation block onto the
// six-part customs valuation. The form records each cost twice (foreign and
// LKR); the foreign figure is the one sent, paired with its currency, matching
// what the endpoint accepts.
func buildValuation(v map[string]any) Valuation {
	currency := str(v, "invoiceCurrencyCode")
	extFreight := nested(v, "externalFreight")

	// Only the invoice and external freight carry their own currency in the
	// form. The rest are declared in the same currency as the invoice.
	amount := func(section string) Amount {
		s := nested(v, section)
		val := number(s, "amountForeign")
		if val == 0 {
			return Amount{}
		}
		return Amount{Value: val, CurrencyID: currency}
	}

	extCurrency := str(extFreight, "currencyCode")
	if extCurrency == "" {
		extCurrency = currency
	}
	external := Amount{}
	if val := number(extFreight, "amountForeign"); val != 0 {
		external = Amount{Value: val, CurrencyID: extCurrency}
	}

	return Valuation{
		ChargeAmount:    Amount{Value: number(v, "invoiceAmountForeign"), CurrencyID: currency},
		ExternalFreight: external,
		InternalFreight: amount("internalFreight"),
		Insurance:       amount("insurance"),
		OtherCost:       amount("otherCost"),
		Deductions:      amount("deduction"),
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

		currency := str(val, "invoiceCurrencyCode")
		if currency == "" {
			currency = str(nested(form, "valuation"), "invoiceCurrencyCode")
		}

		itemValue := number(val, "invoiceAmountForeign")
		if itemValue == 0 {
			itemValue = number(tarif, "itemPrice")
		}

		items = append(items, GoodsItem{
			// Annex A requires a unique item number; position in the array is
			// the only ordering the form has, and it is what numberOfItems and
			// the errors object's segment keys are counted against.
			SequenceNumeric: i + 1,
			CustomsValue: Valuation{
				ChargeAmount: Amount{Value: itemValue, CurrencyID: currency},
			},
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
		})
	}
	return items, nil
}

// massUnit is the unit the form's weight fields are captured in; the form
// labels them in kilograms and offers no unit selector.
const massUnit = "KG"

func buildRemittances(financial map[string]any) []Remittance {
	r := Remittance{
		BankCode:       str(financial, "bankCode"),
		Reference:      str(financial, "bankReference"),
		TermsOfPayment: str(financial, "paymentTermsCode"),
		Amount:         number(financial, "remittanceAmount"),
	}
	// Annex A marks the whole block mandatory, but an empty one carries no
	// information and the endpoint rejects it more clearly than a block of
	// zero values would.
	if r.BankCode == "" && r.Reference == "" && r.TermsOfPayment == "" && r.Amount == 0 {
		return nil
	}
	return []Remittance{r}
}

// buildSupportDocs maps the form's supporting-document rows onto Annex A
// entries. fileName is the storage key rather than the trader's original
// filename: §6.1.2 matches part filenames against supportingDocuments entries
// one-to-one, and two files uploaded under the same original name would make
// that match ambiguous. Keys are unique by construction and keep the uploaded
// extension, so they satisfy both that rule and §8's PDF requirement.
func buildSupportDocs(form map[string]any) ([]SupportDoc, error) {
	raw, ok := form["supportingDocuments"].([]any)
	if !ok || len(raw) == 0 {
		return nil, nil
	}

	docs := make([]SupportDoc, 0, len(raw))
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("customs: supporting document %d is not an object", i+1)
		}

		key := strings.TrimSpace(str(m, "file"))
		if key == "" {
			// A row with a document code but no file would promise an
			// attachment that is not there, which the endpoint rejects (400).
			return nil, fmt.Errorf("customs: supporting document %d has no attached file", i+1)
		}

		docs = append(docs, SupportDoc{
			FileName:       key,
			DocumentCode:   str(m, "documentCode"),
			SequenceNumber: i + 1,
			storageKey:     key,
		})
	}
	return docs, nil
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
