// Package ecdn builds and submits the Electronic Cargo Declaration Note the
// SLPA Cargo Management System (CMS) requires before a Service Order can be
// created for an Export FCL/LCL consignment.
//
// SLPA's CMS is the system of record: it validates the XML against its own
// schema and business rules, detects duplicate declarations, and persists the
// cargo information. NSW generates the document from the trader's form and hands
// it over — it holds no ECDN state of its own beyond what the workflow needs to
// report the outcome back to the trader.
package ecdn

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/OpenNSW/nsw-srilanka/external-integration/slpa/fields"
)

// Declaration is the ECDN document, matching a reference document produced by
// SLPA's own generator at mpma.slpa.lk/ecdn.
//
// Two properties of their schema drive the shape below, and both are easy to
// break by accident:
//
//   - Every element is always present. The reference document carries the unused
//     SUB fields as empty elements rather than omitting them, so nothing here
//     uses omitempty: an XSD sequence treats a missing element and an empty one
//     differently, and omitting them is what a Go struct would do by default.
//   - The document carries no DOCTYPE. Their parser rejects one outright
//     ("Document types are not allowed"), even though their validator also
//     reports "no DTD found" — that message is noise and must not be chased.
type Declaration struct {
	XMLName xml.Name `xml:"CusDecNote"`

	Main       Main       `xml:"MAIN"`
	Sub        Sub        `xml:"SUB"`
	Containers Containers `xml:"CONTAINER"`
}

// Main is the declaration and cargo summary.
type Main struct {
	CusDecNo         string  `xml:"CusDecNo"`
	CusDecDate       string  `xml:"CusDecDate"`
	CusDecOffice     string  `xml:"CusDecOffice"`
	CusDecSerial     string  `xml:"CusDecSerial"`
	Terminal         string  `xml:"Terminal"`
	ShipperName      string  `xml:"ShipperName"`
	ShipperAddress   string  `xml:"ShipperAddress"`
	ConsigneeName    string  `xml:"ConsigneeName"`
	ConsigneeAddress string  `xml:"ConsigneeAddress"`
	PackageNumber    int     `xml:"PackageNumber"`
	PackageType      string  `xml:"PackageType"`
	Volume           float64 `xml:"Volume"`
	Weight           float64 `xml:"Weight"`
	GoodsDescription string  `xml:"GoodsDescription"`
	CusDecSerLet     string  `xml:"CusDecSerLet"`
	Status           string  `xml:"Status"`
}

// Sub is the dispatch and voyage detail.
//
// Most of it is filled in by SLPA rather than by the trader — the arrival,
// declarant, message-sequence and lorry fields are all empty in the reference
// document. They are declared so the elements are still emitted.
type Sub struct {
	CdnNumber       string `xml:"CdnNumber"`
	VoyageNumber    string `xml:"VoyageNumber"`
	VoyageDate      string `xml:"VoyageDate"`
	PortOfLoading   string `xml:"PortOfLoading"`
	PortOfUnloading string `xml:"PortOfUnloading"`
	VesselOPCode    string `xml:"VesselOPCode"`
	ContOPCode      string `xml:"ContOPCode"`
	EXVessel        string `xml:"EXVessel"`

	// Filled in by SLPA on their side; emitted empty.
	SLPANumber     string `xml:"SLPANumber"`
	ArrivalOfficer string `xml:"ArrivalOfficer"`
	ArrivalDate    string `xml:"ArrivalDate"`
	DeclarantCode  string `xml:"DeclarantCode"`
	MsgSeqNo       string `xml:"MsgSeqNo"`
	CdnDate        string `xml:"CdnDate"`
	CdnYear        string `xml:"CdnYear"`
	CdnSerial      string `xml:"CdnSerial"`
	BillNumber     string `xml:"BillNumber"`
	DriverName     string `xml:"DriverName"`
	CleanerName    string `xml:"CleanerName"`
	LorryNumber    string `xml:"LorryNumber"`
	TrailerNumber  string `xml:"TrailerNumber"`
}

// Containers wraps the container lines. The wrapper is upper case and the
// repeated element is lower case, which is theirs to decide, not a typo here.
type Containers struct {
	Container []Container `xml:"container"`
}

// Container is one container line.
type Container struct {
	ContainerNumber string `xml:"ContainerNumber"`
	ContainerType   string `xml:"ContainerType"`
	ContainerSize   string `xml:"ContainerSize"`
	SealNumber      string `xml:"SealNumber"`
	ContainerMark   string `xml:"ContainerMark"`
	Commodity       string `xml:"Commodity"`
}

// BuildXML renders the trader's form as the ECDN document.
//
// Only the two things the trader cannot recover from a server-side message are
// checked here — an unreadable form, and a declaration with no containers, which
// the CMS rejects under both the FCL and LCL rules. Everything else is left to
// the CMS: it is the system of record, and its schema and business-rule errors
// name the field, which a local guess at its rules could not.
func BuildXML(form map[string]any) (string, error) {
	if len(form) == 0 {
		return "", &buildError{"The cargo declaration form could not be read."}
	}

	cusdecNo := fields.String(form, "cusdecNo")
	cusdecDate := fields.String(form, "cusdecDate")
	cusdecOffice := fields.String(form, "cusdecOffice")

	containers, err := buildContainers(form, cusdecNo, cusdecDate)
	if err != nil {
		return "", err
	}

	decl := Declaration{
		Main: Main{
			CusDecNo:     cusdecNo,
			CusDecDate:   cusdecDate,
			CusDecOffice: cusdecOffice,
			// Derived, as SLPA's own generator does: office, declaration number
			// and the declaration's year run together. The trader is not asked
			// for it because it carries no information they hold.
			CusDecSerial:     cusdecSerial(cusdecOffice, cusdecNo, cusdecDate),
			Terminal:         fields.String(form, "terminal"),
			ShipperName:      fields.String(form, "shipperName"),
			ShipperAddress:   fields.String(form, "shipperAddress"),
			ConsigneeName:    fields.String(form, "consigneeName"),
			ConsigneeAddress: fields.String(form, "consigneeAddress"),
			PackageNumber:    fields.Integer(form, "packageNumber"),
			PackageType:      fields.String(form, "packageType"),
			Volume:           fields.Number(form, "volumeCBM"),
			Weight:           fields.Number(form, "weightKg"),
			GoodsDescription: fields.String(form, "goodsDescription"),
			CusDecSerLet:     fields.String(form, "cusdecSerLet"),
			Status:           fields.String(form, "status"),
		},
		Sub: Sub{
			CdnNumber:       fields.String(form, "cdnNumber"),
			VoyageNumber:    fields.String(form, "voyageNumber"),
			VoyageDate:      fields.String(form, "voyageDate"),
			PortOfLoading:   fields.String(form, "portOfLoading"),
			PortOfUnloading: fields.String(form, "portOfUnloading"),
			VesselOPCode:    fields.String(form, "vesselOPCode"),
			ContOPCode:      fields.String(form, "contOPCode"),
			EXVessel:        fields.String(form, "exVessel"),
		},
		Containers: Containers{Container: containers},
	}

	body, err := xml.MarshalIndent(decl, "", "  ")
	if err != nil {
		return "", &buildError{"The cargo declaration could not be converted to XML."}
	}
	// The CMS parses the payload as a document, so it carries its own prolog —
	// and no DOCTYPE, which their parser refuses.
	return xml.Header + string(body), nil
}

// cusdecSerial rebuilds the composite key SLPA's generator writes: the office
// code, the declaration number and the declaration's year, concatenated. Falls
// back to just the parts it has, rather than inventing a year.
func cusdecSerial(office, cusdecNo, cusdecDate string) string {
	year := cusdecDate
	if len(year) >= 4 {
		year = year[:4]
	}
	return office + cusdecNo + year
}

func buildContainers(form map[string]any, cusdecNo, cusdecDate string) ([]Container, error) {
	raw, _ := form["containers"].([]any)
	if len(raw) == 0 {
		return nil, &buildError{"Add at least one container before submitting: SLPA cannot raise a Service Order without one."}
	}

	containers := make([]Container, 0, len(raw))
	for i, entry := range raw {
		m, ok := entry.(map[string]any)
		if !ok {
			return nil, &buildError{fmt.Sprintf("Container %d could not be read.", i+1)}
		}

		number := fields.String(m, "containerNo")
		if number == "" {
			// SLPA's generator writes a per-declaration key here rather than the
			// physical container number, which it carries in ContainerMark. Match
			// that when the trader has not given one of their own.
			number = fmt.Sprintf("%s_%s_%d", cusdecNo, cusdecDate, i+1)
		}

		containers = append(containers, Container{
			ContainerNumber: number,
			// Lower case, as the reference document has it.
			ContainerType: strings.ToLower(fields.String(m, "containerType")),
			ContainerSize: fields.String(m, "containerSize"),
			SealNumber:    fields.String(m, "sealNumber"),
			ContainerMark: fields.String(m, "containerMark"),
			Commodity:     fields.String(m, "commodity"),
		})
	}
	return containers, nil
}

// buildError marks a failure to assemble the declaration, as opposed to a
// transport failure or a CMS rejection. Its message is written for a person: the
// plugin contract BuildRequest implements cannot fail a call, so the reason is
// logged rather than shown, and the trader sees the CMS's answer to the empty
// declaration that gets sent instead (see Interpreter.BuildRequest).
type buildError struct{ msg string }

func (e *buildError) Error() string { return e.msg }
