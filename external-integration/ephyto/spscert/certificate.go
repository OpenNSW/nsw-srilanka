package spscert

import "encoding/xml"

// ===========================================================================
// XML MODEL  (rsm:SPSCertificate, UN/CEFACT SPSCertificate package 17A)
//
// We use literal namespace-prefixed tag names ("ram:TypeCode") plus explicit
// xmlns:* attributes on the root. This is the reliable way to get fixed, clean
// prefixes out of encoding/xml. Field ORDER == element order in the output, so
// the struct field order below IS the schema sequence we emit.
// ===========================================================================

// UN/CEFACT SPSCertificate package 17A namespaces.
const (
	nsRSM = "urn:un:unece:uncefact:data:standard:SPSCertificate:17"
	nsRAM = "urn:un:unece:uncefact:data:standard:ReusableAggregateBusinessInformationEntity:21"
	nsUDT = "urn:un:unece:uncefact:data:standard:UnqualifiedDataType:21"
	nsQDT = "urn:un:unece:uncefact:data:standard:QualifiedDataType:21"
)

type SPSCertificate struct {
	XMLName     xml.Name          `xml:"rsm:SPSCertificate"`
	XmlnsRSM    string            `xml:"xmlns:rsm,attr"`
	XmlnsRAM    string            `xml:"xmlns:ram,attr"`
	XmlnsUDT    string            `xml:"xmlns:udt,attr"`
	XmlnsQDT    string            `xml:"xmlns:qdt,attr"`
	Document    ExchangedDocument `xml:"rsm:SPSExchangedDocument"`
	Consignment Consignment       `xml:"rsm:SPSConsignment"`
}

type ExchangedDocument struct {
	Name          string        `xml:"ram:Name"` // NON-PC, present for schema (empty)
	ID            string        `xml:"ram:ID"`
	TypeCode      string        `xml:"ram:TypeCode"`
	StatusCode    string        `xml:"ram:StatusCode"`
	IssueDateTime DateTime      `xml:"ram:IssueDateTime"`
	Issuer        NameParty     `xml:"ram:IssuerSPSParty"`
	Notes         []Note        `xml:"ram:IncludedSPSNote,omitempty"`
	Signatory     Signatory     `xml:"ram:SignatorySPSAuthentication"`
	Attachments   []RefDocument `xml:"ram:ReferenceSPSReferencedDocument,omitempty"`
}

type NameParty struct {
	Name string `xml:"ram:Name"`
}

type DateTime struct {
	DateTimeString string `xml:"udt:DateTimeString"`
}

type Signatory struct {
	ActualDate    DateTime      `xml:"ram:ActualDateTime"`
	IssueLocation NameParty     `xml:"ram:IssueSPSLocation"`
	Provider      ProviderParty `xml:"ram:ProviderSPSParty"`
	Clauses       []Clause      `xml:"ram:IncludedSPSClause"`
}

type ProviderParty struct {
	Name   string     `xml:"ram:Name"` // NON-PC empty
	Person PersonName `xml:"ram:SpecifiedSPSPerson"`
}

type PersonName struct {
	Name string `xml:"ram:Name"`
}

type Clause struct {
	ID      string `xml:"ram:ID,omitempty"`
	Content string `xml:"ram:Content"`
}

// Note == ram:IncludedSPSNote OR ram:AdditionalInformationSPSNote (identical
// shape). Subject is the fixed code (ADEDL, DMCL, RPCST, OQV, ...). Contents
// allows multilingual repetition of <ram:Content>.
type Note struct {
	Subject  string        `xml:"ram:Subject"`
	Contents []NoteContent `xml:"ram:Content"`
}

type NoteContent struct {
	Lang  string `xml:"languageID,attr,omitempty"`
	Value string `xml:",chardata"`
}

type RefDocument struct {
	IssueDateTime        string        `xml:"ram:IssueDateTime,omitempty"`
	RelationshipTypeCode string        `xml:"ram:RelationshipTypeCode"`
	ID                   string        `xml:"ram:ID"`
	Attachment           *BinaryObject `xml:"ram:AttachmentBinaryObject,omitempty"`
	Information          *LangText     `xml:"ram:Information,omitempty"`
}

type BinaryObject struct {
	Filename string `xml:"filename,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type Consignment struct {
	Consignor        Party           `xml:"ram:ConsignorSPSParty"`
	Consignee        Party           `xml:"ram:ConsigneeSPSParty"`
	ExportCountry    CountryIDName   `xml:"ram:ExportSPSCountry"`
	ImportCountry    CountryIDName   `xml:"ram:ImportSPSCountry"`
	TransitCountries []CountryIDName `xml:"ram:TransitSPSCountry,omitempty"`
	PointOfEntry     *Location       `xml:"ram:UnloadingBaseportSPSLocation,omitempty"`
	Examination      Examination     `xml:"ram:ExaminationSPSEvent"`
	Conveyances      []TransportMove `xml:"ram:MainCarriageSPSTransportMovement,omitempty"`
	Equipment        *Equipment      `xml:"ram:UtilizedSPSTransportEquipment,omitempty"`
	Items            []SPSItem       `xml:"ram:IncludedSPSConsignmentItem"`
}

type Examination struct {
	OccurrenceLocation NameParty `xml:"ram:OccurrenceSPSLocation"`
}

type CountryIDName struct {
	ID   string `xml:"ram:ID"`
	Name string `xml:"ram:Name"` // NON-PC empty
}

type Party struct {
	Name     string         `xml:"ram:Name"`
	TypeCode *PartyTypeCode `xml:"ram:TypeCode,omitempty"`
	Address  *Address       `xml:"ram:SpecifiedSPSAddress,omitempty"`
}

type PartyTypeCode struct {
	ListName string `xml:"listName,attr,omitempty"`
	Value    string `xml:",chardata"`
}

type Address struct {
	LineOne   string `xml:"ram:LineOne"`
	LineTwo   string `xml:"ram:LineTwo,omitempty"`
	LineThree string `xml:"ram:LineThree,omitempty"`
	LineFour  string `xml:"ram:LineFour,omitempty"`
	LineFive  string `xml:"ram:LineFive,omitempty"`
}

type Equipment struct {
	ID   string `xml:"ram:ID"`
	Seal *Seal  `xml:"ram:AffixedSPSSeal,omitempty"`
}

type Seal struct {
	ID string `xml:"ram:ID"`
}

type Location struct {
	ID   string `xml:"ram:ID,omitempty"`
	Name string `xml:"ram:Name"`
}

type TransportMove struct {
	ID       string         `xml:"ram:ID,omitempty"`
	ModeCode string         `xml:"ram:ModeCode"`
	Means    *TransportMean `xml:"ram:UsedSPSTransportMeans,omitempty"`
}

type TransportMean struct {
	Name LangText `xml:"ram:Name"`
}

type LangText struct {
	Lang  string `xml:"languageID,attr,omitempty"`
	Value string `xml:",chardata"`
}

type SPSItem struct {
	TradeLines []TradeLine `xml:"ram:IncludedSPSTradeLineItem"`
}

// TradeLine field order is the emitted element order. AdditionalInformationSPSNote
// appears at three logical positions per the mapping document (packaging /
// re-export, quantity, then declarations); each is a separate slice so position
// is preserved. See MarshalXML for why the custom marshaller is required.
type TradeLine struct {
	Sequence         string            `xml:"ram:SequenceNumeric"`
	Descriptions     []LangText        `xml:"ram:Description"` // MANDATORY
	CommonNames      []LangText        `xml:"ram:CommonName,omitempty"`
	ScientificName   string            `xml:"ram:ScientificName,omitempty"`
	IntendedUses     []LangText        `xml:"ram:IntendedUse,omitempty"`
	NetWeight        *Measure          `xml:"ram:NetWeightMeasure,omitempty"`
	GrossWeight      *Measure          `xml:"ram:GrossWeightMeasure,omitempty"`
	NetVolume        *Measure          `xml:"ram:NetVolumeMeasure,omitempty"`
	GrossVolume      *Measure          `xml:"ram:GrossVolumeMeasure,omitempty"`
	PackagingNotes   []Note            `xml:"ram:AdditionalInformationSPSNote,omitempty"` // OPTND + RPC* re-export block
	QuantityNotes    []Note            `xml:"ram:AdditionalInformationSPSNote,omitempty"` // OQV/OQU
	DeclarationNotes []Note            `xml:"ram:AdditionalInformationSPSNote,omitempty"` // DMTLIL/ADTLIL/ADIPTLIL/ADDITLIL/ADAOTLIL
	Classifications  []Classification  `xml:"ram:ApplicableSPSClassification,omitempty"`
	Packages         []PhysicalPackage `xml:"ram:PhysicalSPSPackage,omitempty"`
	Origins          []OriginCountry   `xml:"ram:OriginSPSCountry"`
	Treatments       []AppliedProcess  `xml:"ram:AppliedSPSProcess,omitempty"`
}

// MarshalXML emits the trade-line children in explicit document order.
// A custom marshaller is required because three fields map to the same element
// name (ram:AdditionalInformationSPSNote); Go's reflect-based encoder would
// treat them as conflicting and drop all three.
func (t TradeLine) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	enc := func(name string, v interface{}) error {
		return e.EncodeElement(v, xml.StartElement{Name: xml.Name{Local: name}})
	}
	encNotes := func(notes []Note) error {
		for _, n := range notes {
			if err := enc("ram:AdditionalInformationSPSNote", n); err != nil {
				return err
			}
		}
		return nil
	}

	// 1. Sequence Numeric
	if err := enc("ram:SequenceNumeric", t.Sequence); err != nil {
		return err
	}

	// 2. Descriptions
	for _, d := range t.Descriptions {
		if err := enc("ram:Description", d); err != nil {
			return err
		}
	}

	// 3. Common Names
	for _, cn := range t.CommonNames {
		if err := enc("ram:CommonName", cn); err != nil {
			return err
		}
	}

	// 4. Scientific Name
	if t.ScientificName != "" {
		if err := enc("ram:ScientificName", t.ScientificName); err != nil {
			return err
		}
	}

	// 5. Intended Uses
	for _, iu := range t.IntendedUses {
		if err := enc("ram:IntendedUse", iu); err != nil {
			return err
		}
	}

	// 6. Net/Gross Quantities
	if t.NetWeight != nil {
		if err := enc("ram:NetWeightMeasure", t.NetWeight); err != nil {
			return err
		}
	}
	if t.GrossWeight != nil {
		if err := enc("ram:GrossWeightMeasure", t.GrossWeight); err != nil {
			return err
		}
	}
	if t.NetVolume != nil {
		if err := enc("ram:NetVolumeMeasure", t.NetVolume); err != nil {
			return err
		}
	}
	if t.GrossVolume != nil {
		if err := enc("ram:GrossVolumeMeasure", t.GrossVolume); err != nil {
			return err
		}
	}

	// 7. AdditionalInformationSPSNotes (Packaging, Quantity, and Declaration Notes combined)
	if err := encNotes(t.PackagingNotes); err != nil {
		return err
	}
	if err := encNotes(t.QuantityNotes); err != nil {
		return err
	}
	if err := encNotes(t.DeclarationNotes); err != nil {
		return err
	}

	// 8. Classifications
	for _, c := range t.Classifications {
		if err := enc("ram:ApplicableSPSClassification", c); err != nil {
			return err
		}
	}

	// 9. Packages
	for _, p := range t.Packages {
		if err := enc("ram:PhysicalSPSPackage", p); err != nil {
			return err
		}
	}

	// 10. Origin Countries
	for _, o := range t.Origins {
		if err := enc("ram:OriginSPSCountry", o); err != nil {
			return err
		}
	}

	// 11. Treatments
	for _, tr := range t.Treatments {
		if err := enc("ram:AppliedSPSProcess", tr); err != nil {
			return err
		}
	}

	return e.EncodeToken(xml.EndElement{Name: start.Name})
}

type PhysicalPackage struct {
	LevelCode    string `xml:"ram:LevelCode"`
	TypeCode     string `xml:"ram:TypeCode"`
	ItemQuantity string `xml:"ram:ItemQuantity"`
}

type OriginCountry struct {
	ID          string       `xml:"ram:ID"`
	Name        string       `xml:"ram:Name"` // NON-PC empty
	Subdivision *Subdivision `xml:"ram:SubordinateSPSCountrySubDivision,omitempty"`
}

type Subdivision struct {
	Name                  string `xml:"ram:Name"`
	HierarchicalLevelCode string `xml:"ram:HierarchicalLevelCode"`
}

type Measure struct {
	UnitCode string `xml:"unitCode,attr"`
	Value    string `xml:",chardata"`
}

type Classification struct {
	SystemName string    `xml:"ram:SystemName"`
	ClassCode  string    `xml:"ram:ClassCode,omitempty"`
	ClassName  *LangText `xml:"ram:ClassName,omitempty"`
}

type AppliedProcess struct {
	TypeCode         string                  `xml:"ram:TypeCode"` // MUST be ZZZ for ePhyto validation
	CompletionPeriod *CompletionPeriod       `xml:"ram:CompletionSPSPeriod,omitempty"`
	Characteristics  []ProcessCharacteristic `xml:"ram:ApplicableSPSProcessCharacteristic,omitempty"`
}

type CompletionPeriod struct {
	Start    *DateTime `xml:"ram:StartDateTime,omitempty"`
	End      *DateTime `xml:"ram:EndDateTime,omitempty"`
	Duration *Measure  `xml:"ram:DurationMeasure,omitempty"`
}

type ProcessCharacteristic struct {
	Descriptions []LangText `xml:"ram:Description"`
	ValueMeasure *Measure   `xml:"ram:ValueMeasure,omitempty"`
}
