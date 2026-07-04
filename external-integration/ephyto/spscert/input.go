// Package spscert holds the ePhyto content layer: the friendly JSON input
// contract (this file), the rigid UN/CEFACT SPSCertificate XML model
// (certificate.go), and the mapping between them (convert.go).
package spscert

// ===========================================================================
// INPUT MODEL  (the friendly JSON the national system produces)
//
// These structs are the JSON contract. They are deliberately decoupled from the
// XML model in certificate.go; convert.go maps one to the other. Edit field
// names/tags here to change the JSON shape without touching the XML.
// ===========================================================================

// Input is the top-level JSON document.
type Input struct {
	SOAP        SOAPInput `json:"soap"`
	Certificate CertInput `json:"certificate"`
}

// SOAPInput carries the Hub envelope header. From/To are ISO 3166-1 alpha-2
// country codes.
type SOAPInput struct {
	Operation             string   `json:"operation"`             // default: ValidateAndDeliverEnvelope
	From                  string   `json:"from"`                  // e.g. "LK"
	To                    string   `json:"to"`                    // e.g. "DE"
	CertificateType       int      `json:"certificateType"`       // 851 = Phyto, 657 = Re-export Phyto
	CertificateStatus     int      `json:"certificateStatus"`     // e.g. 70
	NPPOCertificateNumber string   `json:"nppoCertificateNumber"` // your internal cert number
	Forwardings           []string `json:"forwardings,omitempty"` // optional channel codes
}

// CertInput is the ePhyto content.
type CertInput struct {
	TypeCode               string           `json:"typeCode"`               // "851" (PC) or "657" (PC/R)
	Number                 string           `json:"certificateNumber"`      // SPSExchangedDocument.ID
	IssuingNPPO            string           `json:"issuingNPPO"`            // issuer name
	StatusCode             string           `json:"statusCode"`             // e.g. "70"
	IssueDateTime          string           `json:"issueDateTime"`          // W3 e.g. 2026-05-21T10:30:00+05:30
	AuthorizedOfficer      string           `json:"authorizedOfficer"`      // signatory person name
	PlaceOfIssue           string           `json:"placeOfIssue"`           // signatory location name
	CertifyingStatementIDs []string         `json:"certifyingStatementIDs"` // ["1","2"] for PC; empty for PC/R
	FinancialLiability     string           `json:"financialLiability,omitempty"`
	DocDeclarations        *DocDeclarations `json:"documentLevelDeclarations,omitempty"`
	Replacement            *Replacement     `json:"replacement,omitempty"`
	Attachments            []Attachment     `json:"attachments,omitempty"`
	Consignment            ConsignmentInput `json:"consignment"`
}

// DocDeclarations are document-level IncludedSPSNote entries. Use these only
// when the statement applies to the WHOLE consignment; otherwise put them at
// the trade-line level.
type DocDeclarations struct {
	AdditionalDeclarations []string `json:"additionalDeclarations,omitempty"` // ADEDL
	ImportPermit           string   `json:"importPermit,omitempty"`           // ADIPEDL
	DateOfInspection       string   `json:"dateOfInspection,omitempty"`       // ADDIEDL
	AdditionalOfficialInfo []string `json:"additionalOfficialInfo,omitempty"` // ADAOEDL
	DistinguishingMarks    []string `json:"distinguishingMarks,omitempty"`    // DMCL
	LanguageID             string   `json:"languageID,omitempty"`             // default for the above text notes
}

// Replacement populates the ADRP / ADRPN / ADRPR / ADRD document-level notes.
type Replacement struct {
	StatementCode     string `json:"statementCode"`     // ADRP content, e.g. "4"
	ReplacedNumber    string `json:"replacedNumber"`    // ADRPN
	Reason            string `json:"reason"`            // ADRPR
	ReplacedIssueDate string `json:"replacedIssueDate"` // ADRD (W3)
	LanguageID        string `json:"languageID,omitempty"`
}

// Attachment maps to ram:ReferenceSPSReferencedDocument. base64 is the file
// content; for the original-certificate copy use relationshipTypeCode "AWR",
// otherwise "ZZZ".
type Attachment struct {
	RelationshipTypeCode string `json:"relationshipTypeCode"` // ZZZ (default) or AWR
	ID                   string `json:"id"`
	Filename             string `json:"filename,omitempty"`
	Base64               string `json:"base64,omitempty"`
	Information          string `json:"information,omitempty"`
	IssueDate            string `json:"issueDate,omitempty"`
	LanguageID           string `json:"languageID,omitempty"`
}

type ConsignmentInput struct {
	ExportCountry     string        `json:"exportCountry"` // ISO code, issuing NPPO country
	ImportCountry     string        `json:"importCountry"` // ISO code
	TransitCountries  []string      `json:"transitCountries,omitempty"`
	Consignor         PartyInput    `json:"consignor"`
	Consignee         PartyInput    `json:"consignee"`
	Seal              string        `json:"transportEquipmentSeal,omitempty"`
	PointOfEntry      *PointOfEntry `json:"pointOfEntry,omitempty"`
	MeansOfConveyance []Conveyance  `json:"meansOfConveyance,omitempty"`
	Items             []ItemInput   `json:"items"`
}

type PartyInput struct {
	Name         string   `json:"name"`
	AddressLines []string `json:"addressLines,omitempty"`
	IDType       string   `json:"idTypeListName,omitempty"` // optional TypeCode listName
	IDValue      string   `json:"idValue,omitempty"`        // optional TypeCode value
}

type PointOfEntry struct {
	Locode string `json:"locode,omitempty"` // UN/LOCODE, e.g. DEHAM
	Name   string `json:"name"`
}

type Conveyance struct {
	ID         string `json:"id,omitempty"`   // voyage / flight number
	ModeCode   string `json:"modeCode"`       // IPPC mode-of-transport numeric code (e.g. "1" maritime)
	Name       string `json:"name,omitempty"` // carrier / vessel name
	LanguageID string `json:"languageID,omitempty"`
}

type ItemInput struct {
	TradeLines []TradeLineInput `json:"tradeLines"`
}

type TradeLineInput struct {
	Sequence            int            `json:"sequence"`
	Description         string         `json:"description"` // MANDATORY (UN/CEFACT)
	DescriptionLang     string         `json:"descriptionLanguageID,omitempty"`
	CommonNames         []string       `json:"commonNames,omitempty"`
	ScientificName      string         `json:"scientificName,omitempty"`
	OriginCountries     []string       `json:"originCountries"`
	HSCode              string         `json:"hsCode,omitempty"`
	VegetablePart       string         `json:"vegetablePart,omitempty"` // IPPCPCVP
	Condition           string         `json:"condition,omitempty"`     // IPPCPCC
	IntendedUse         string         `json:"intendedUse,omitempty"`
	Packages            []PackageInput `json:"packages,omitempty"`
	PackageDescription  string         `json:"packageDescription,omitempty"` // OPTND fallback
	NetWeight           *MeasureInput  `json:"netWeight,omitempty"`
	GrossWeight         *MeasureInput  `json:"grossWeight,omitempty"`
	NetVolume           *MeasureInput  `json:"netVolume,omitempty"`
	GrossVolume         *MeasureInput  `json:"grossVolume,omitempty"`
	OtherQuantity       *OtherQuantity `json:"otherQuantity,omitempty"`
	DistinguishingMarks string         `json:"distinguishingMarks,omitempty"`    // DMTLIL
	AdditionalDecls     []string       `json:"additionalDeclarations,omitempty"` // ADTLIL
	ImportPermit        string         `json:"importPermit,omitempty"`           // ADIPTLIL
	DateOfInspection    string         `json:"dateOfInspection,omitempty"`       // ADDITLIL
	AdditionalInfo      []string       `json:"additionalOfficialInfo,omitempty"` // ADAOTLIL
	LanguageID          string         `json:"languageID,omitempty"`             // default for text notes
	Treatments          []Treatment    `json:"treatments,omitempty"`
	ReExport            *ReExport      `json:"reExport,omitempty"`
}

type PackageInput struct {
	LevelCode string `json:"levelCode"` // "1", "2", ...
	TypeCode  string `json:"typeCode"`  // IPPC package code, e.g. BG, BX
	Quantity  string `json:"quantity"`  // numeric, 4 decimals recommended
}

type MeasureInput struct {
	Value string `json:"value"`
	Unit  string `json:"unit"` // KGM, MTQ, ...
}

type OtherQuantity struct {
	Value string `json:"value"` // OQV
	Unit  string `json:"unit"`  // OQU (free text up to 50 chars)
}

type Treatment struct {
	StartDate      string        `json:"startDate,omitempty"`
	EndDate        string        `json:"endDate,omitempty"`
	TypeLevel1     string        `json:"treatmentTypeL1,omitempty"` // TTL1 (code or text)
	TypeLevel2     string        `json:"treatmentTypeL2,omitempty"` // TTL2
	Chemical       string        `json:"chemical,omitempty"`        // TTCH active ingredient code/text
	Duration       *MeasureInput `json:"duration,omitempty"`        // unit e.g. HUR
	Temperature    *MeasureInput `json:"temperature,omitempty"`     // TTTM, unit e.g. CEL
	Concentration  *MeasureInput `json:"concentration,omitempty"`   // TTCO, unit e.g. KX
	AdditionalInfo string        `json:"additionalInfo,omitempty"`  // TTAI
	FullTreatment  string        `json:"fullTreatment"`             // TTFT (always include)
	LanguageID     string        `json:"languageID,omitempty"`
}

// ReExport drives the re-export certifying statements (subjects RPC*) used only
// for 657 / PC-R certificates.
type ReExport struct {
	StatementCode        string   `json:"statementCode"`        // RPCST, e.g. "3"
	CountriesOfOrigin    []string `json:"countriesOfOrigin"`    // RPCCO (ISO codes)
	OriginalCertRefs     []string `json:"originalCertRefs"`     // RPCRF
	IsOriginal           bool     `json:"isOriginal"`           // RPCOR
	CertifiedTrueCopy    bool     `json:"certifiedTrueCopy"`    // RPCTC
	Packed               bool     `json:"packed"`               // RPCPK
	Repacked             bool     `json:"repacked"`             // RPCRP
	OriginalContainers   bool     `json:"originalContainers"`   // RPCOC
	NewContainers        bool     `json:"newContainers"`        // RPCNC
	OriginalPCAttached   bool     `json:"originalPCAttached"`   // RPCPC (must be true)
	AdditionalInspection bool     `json:"additionalInspection"` // RPCAI
}
