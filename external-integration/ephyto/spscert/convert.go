package spscert

import (
	"fmt"
	"strings"
)

// ===========================================================================
// CONVERSION: Input (JSON) -> SPSCertificate (XML model)
//
// This is the only place that knows how the friendly JSON maps onto the rigid
// 17A structure (e.g. how a re-export boolean becomes an RPC* note, or how a
// document-level declaration becomes an IncludedSPSNote with a fixed Subject).
// ===========================================================================

func boolStr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

func textNote(subject, content, lang string) Note {
	return Note{Subject: subject, Contents: []NoteContent{{Lang: lang, Value: content}}}
}

func BuildCertificate(in Input) (SPSCertificate, error) {
	c := in.Certificate

	if strings.TrimSpace(c.Number) == "" {
		return SPSCertificate{}, fmt.Errorf("certificate.certificateNumber is required")
	}
	if strings.TrimSpace(c.IssueDateTime) == "" {
		return SPSCertificate{}, fmt.Errorf("certificate.issueDateTime is required (W3 datetime)")
	}
	if len(c.Consignment.Items) == 0 {
		return SPSCertificate{}, fmt.Errorf("certificate.consignment.items must have at least one item")
	}

	// --- Document level ---
	doc := ExchangedDocument{
		TypeCode:      c.TypeCode,
		Name:          "",
		ID:            c.Number,
		Issuer:        NameParty{Name: c.IssuingNPPO},
		StatusCode:    c.StatusCode,
		IssueDateTime: DateTime{DateTimeString: c.IssueDateTime},
	}

	// Signatory + certifying statements.
	var clauses []Clause
	if len(c.CertifyingStatementIDs) == 0 {
		// PC/R (657): single empty clause for schema compliance.
		clauses = append(clauses, Clause{Content: ""})
	} else {
		for _, id := range c.CertifyingStatementIDs {
			clauses = append(clauses, Clause{ID: id, Content: ""})
		}
	}
	doc.Signatory = Signatory{
		Provider:      ProviderParty{Person: PersonName{Name: c.AuthorizedOfficer}, Name: ""},
		IssueLocation: NameParty{Name: c.PlaceOfIssue},
		ActualDate:    DateTime{DateTimeString: ""},
		Clauses:       clauses,
	}

	// Attachments.
	for _, a := range c.Attachments {
		rel := a.RelationshipTypeCode
		if rel == "" {
			rel = "ZZZ"
		}
		rd := RefDocument{
			IssueDateTime:        a.IssueDate,
			RelationshipTypeCode: rel,
			ID:                   a.ID,
		}
		if a.Base64 != "" || a.Filename != "" {
			rd.Attachment = &BinaryObject{Filename: a.Filename, Value: a.Base64}
		}
		if a.Information != "" {
			rd.Information = &LangText{Lang: a.LanguageID, Value: a.Information}
		}
		doc.Attachments = append(doc.Attachments, rd)
	}

	// Document-level notes (order: SPSFL, ADEDL, ADIPEDL, ADDIEDL, ADAOEDL,
	// replacement block, DMCL).
	if c.FinancialLiability != "" {
		doc.Notes = append(doc.Notes, textNote("SPSFL", c.FinancialLiability, ""))
	}
	if d := c.DocDeclarations; d != nil {
		for _, t := range d.AdditionalDeclarations {
			doc.Notes = append(doc.Notes, textNote("ADEDL", t, d.LanguageID))
		}
		if d.ImportPermit != "" {
			doc.Notes = append(doc.Notes, textNote("ADIPEDL", d.ImportPermit, ""))
		}
		if d.DateOfInspection != "" {
			doc.Notes = append(doc.Notes, textNote("ADDIEDL", d.DateOfInspection, ""))
		}
		for _, t := range d.AdditionalOfficialInfo {
			doc.Notes = append(doc.Notes, textNote("ADAOEDL", t, d.LanguageID))
		}
		for _, t := range d.DistinguishingMarks {
			doc.Notes = append(doc.Notes, textNote("DMCL", t, ""))
		}
	}
	if r := c.Replacement; r != nil {
		if r.StatementCode != "" {
			doc.Notes = append(doc.Notes, textNote("ADRP", r.StatementCode, ""))
		}
		if r.ReplacedNumber != "" {
			doc.Notes = append(doc.Notes, textNote("ADRPN", r.ReplacedNumber, ""))
		}
		if r.Reason != "" {
			doc.Notes = append(doc.Notes, textNote("ADRPR", r.Reason, r.LanguageID))
		}
		if r.ReplacedIssueDate != "" {
			doc.Notes = append(doc.Notes, textNote("ADRD", r.ReplacedIssueDate, ""))
		}
	}

	// --- Consignment level ---
	cons := in.Certificate.Consignment
	con := Consignment{
		Examination:   Examination{OccurrenceLocation: NameParty{Name: ""}},
		ExportCountry: CountryIDName{ID: cons.ExportCountry},
		ImportCountry: CountryIDName{ID: cons.ImportCountry},
		Consignor:     buildParty(cons.Consignor),
		Consignee:     buildParty(cons.Consignee),
	}
	for _, t := range cons.TransitCountries {
		con.TransitCountries = append(con.TransitCountries, CountryIDName{ID: t})
	}
	if cons.Seal != "" {
		con.Equipment = &Equipment{ID: "", Seal: &Seal{ID: cons.Seal}}
	}
	if cons.PointOfEntry != nil {
		con.PointOfEntry = &Location{ID: cons.PointOfEntry.Locode, Name: cons.PointOfEntry.Name}
	}
	for _, m := range cons.MeansOfConveyance {
		tm := TransportMove{ID: m.ID, ModeCode: m.ModeCode}
		if m.Name != "" {
			tm.Means = &TransportMean{Name: LangText{Lang: m.LanguageID, Value: m.Name}}
		}
		con.Conveyances = append(con.Conveyances, tm)
	}

	for _, item := range cons.Items {
		var spsItem SPSItem
		for _, tl := range item.TradeLines {
			built, err := buildTradeLine(tl)
			if err != nil {
				return SPSCertificate{}, err
			}
			spsItem.TradeLines = append(spsItem.TradeLines, built)
		}
		con.Items = append(con.Items, spsItem)
	}

	return SPSCertificate{
		XmlnsRSM:    nsRSM,
		XmlnsRAM:    nsRAM,
		XmlnsUDT:    nsUDT,
		XmlnsQDT:    nsQDT,
		Document:    doc,
		Consignment: con,
	}, nil
}

func buildParty(p PartyInput) Party {
	party := Party{Name: p.Name}
	if p.IDValue != "" {
		party.TypeCode = &PartyTypeCode{ListName: p.IDType, Value: p.IDValue}
	}
	if len(p.AddressLines) > 0 {
		addr := &Address{LineOne: p.AddressLines[0]}
		if len(p.AddressLines) > 1 {
			addr.LineTwo = p.AddressLines[1]
		}
		if len(p.AddressLines) > 2 {
			addr.LineThree = p.AddressLines[2]
		}
		if len(p.AddressLines) > 3 {
			addr.LineFour = p.AddressLines[3]
		}
		if len(p.AddressLines) > 4 {
			addr.LineFive = p.AddressLines[4]
		}
		party.Address = addr
	}
	return party
}

func buildTradeLine(tl TradeLineInput) (TradeLine, error) {
	if strings.TrimSpace(tl.Description) == "" {
		return TradeLine{}, fmt.Errorf("each tradeLine.description is mandatory (UN/CEFACT)")
	}
	if len(tl.OriginCountries) == 0 {
		return TradeLine{}, fmt.Errorf("each tradeLine needs at least one originCountries entry")
	}

	out := TradeLine{Sequence: fmt.Sprintf("%d", tl.Sequence)}

	// Packaging: standard PhysicalSPSPackage OR the OPTND fallback note. The
	// mapping requires exactly one of the two to be present.
	if len(tl.Packages) > 0 {
		for _, p := range tl.Packages {
			out.Packages = append(out.Packages, PhysicalPackage{
				LevelCode:    p.LevelCode,
				TypeCode:     p.TypeCode,
				ItemQuantity: p.Quantity,
			})
		}
	} else if tl.PackageDescription != "" {
		out.PackagingNotes = append(out.PackagingNotes, textNote("OPTND", tl.PackageDescription, ""))
	}

	// Re-export certifying statements (657 only).
	if r := tl.ReExport; r != nil {
		if r.StatementCode != "" {
			out.PackagingNotes = append(out.PackagingNotes, textNote("RPCST", r.StatementCode, ""))
		}
		for _, co := range r.CountriesOfOrigin {
			out.PackagingNotes = append(out.PackagingNotes, textNote("RPCCO", co, ""))
		}
		for _, ref := range r.OriginalCertRefs {
			out.PackagingNotes = append(out.PackagingNotes, textNote("RPCRF", ref, ""))
		}
		out.PackagingNotes = append(out.PackagingNotes,
			textNote("RPCOR", boolStr(r.IsOriginal), ""),
			textNote("RPCTC", boolStr(r.CertifiedTrueCopy), ""),
			textNote("RPCPK", boolStr(r.Packed), ""),
			textNote("RPCRP", boolStr(r.Repacked), ""),
			textNote("RPCOC", boolStr(r.OriginalContainers), ""),
			textNote("RPCNC", boolStr(r.NewContainers), ""),
			textNote("RPCPC", boolStr(r.OriginalPCAttached), ""),
			textNote("RPCAI", boolStr(r.AdditionalInspection), ""),
		)
	}

	// Origins.
	for _, oc := range tl.OriginCountries {
		out.Origins = append(out.Origins, OriginCountry{ID: oc})
	}

	// Description (mandatory) + common/scientific names.
	out.Descriptions = []LangText{{Lang: tl.DescriptionLang, Value: tl.Description}}
	for _, cn := range tl.CommonNames {
		out.CommonNames = append(out.CommonNames, LangText{Lang: tl.LanguageID, Value: cn})
	}
	out.ScientificName = tl.ScientificName

	// Quantities.
	if tl.NetWeight != nil {
		out.NetWeight = &Measure{UnitCode: tl.NetWeight.Unit, Value: tl.NetWeight.Value}
	}
	if tl.GrossWeight != nil {
		out.GrossWeight = &Measure{UnitCode: tl.GrossWeight.Unit, Value: tl.GrossWeight.Value}
	}
	if tl.NetVolume != nil {
		out.NetVolume = &Measure{UnitCode: tl.NetVolume.Unit, Value: tl.NetVolume.Value}
	}
	if tl.GrossVolume != nil {
		out.GrossVolume = &Measure{UnitCode: tl.GrossVolume.Unit, Value: tl.GrossVolume.Value}
	}
	if tl.OtherQuantity != nil {
		out.QuantityNotes = append(out.QuantityNotes,
			textNote("OQV", tl.OtherQuantity.Value, ""),
			textNote("OQU", tl.OtherQuantity.Unit, ""),
		)
	}

	// Classifications: HS (ClassName present but empty), IPPCPCVP, IPPCPCC.
	if tl.HSCode != "" {
		out.Classifications = append(out.Classifications, Classification{
			SystemName: "HS",
			ClassCode:  tl.HSCode,
			ClassName:  &LangText{Value: ""}, // required-but-empty per mapping
		})
	}
	if tl.VegetablePart != "" {
		out.Classifications = append(out.Classifications, Classification{
			SystemName: "IPPCPCVP",
			ClassName:  &LangText{Lang: tl.LanguageID, Value: tl.VegetablePart},
		})
	}
	if tl.Condition != "" {
		out.Classifications = append(out.Classifications, Classification{
			SystemName: "IPPCPCC",
			ClassName:  &LangText{Lang: tl.LanguageID, Value: tl.Condition},
		})
	}
	if tl.IntendedUse != "" {
		out.IntendedUses = append(out.IntendedUses, LangText{Lang: tl.LanguageID, Value: tl.IntendedUse})
	}

	// Trade-line declarations.
	if tl.DistinguishingMarks != "" {
		out.DeclarationNotes = append(out.DeclarationNotes, textNote("DMTLIL", tl.DistinguishingMarks, ""))
	}
	for _, t := range tl.AdditionalDecls {
		out.DeclarationNotes = append(out.DeclarationNotes, textNote("ADTLIL", t, tl.LanguageID))
	}
	if tl.ImportPermit != "" {
		out.DeclarationNotes = append(out.DeclarationNotes, textNote("ADIPTLIL", tl.ImportPermit, ""))
	}
	if tl.DateOfInspection != "" {
		out.DeclarationNotes = append(out.DeclarationNotes, textNote("ADDITLIL", tl.DateOfInspection, ""))
	}
	for _, t := range tl.AdditionalInfo {
		out.DeclarationNotes = append(out.DeclarationNotes, textNote("ADAOTLIL", t, tl.LanguageID))
	}

	// Treatments.
	for _, tr := range tl.Treatments {
		out.Treatments = append(out.Treatments, buildTreatment(tr))
	}

	return out, nil
}

func buildTreatment(tr Treatment) AppliedProcess {
	ap := AppliedProcess{TypeCode: "ZZZ"} // ZZZ is required for ePhyto validation

	// CompletionSPSPeriod groups start/end dates AND the duration measure.
	if tr.StartDate != "" || tr.EndDate != "" || tr.Duration != nil {
		cp := &CompletionPeriod{}
		if tr.StartDate != "" {
			cp.Start = &DateTime{DateTimeString: tr.StartDate}
		}
		if tr.EndDate != "" {
			cp.End = &DateTime{DateTimeString: tr.EndDate}
		}
		if tr.Duration != nil {
			cp.Duration = &Measure{UnitCode: tr.Duration.Unit, Value: tr.Duration.Value}
		}
		ap.CompletionPeriod = cp
	}

	add := func(descs []LangText, vm *Measure) {
		ap.Characteristics = append(ap.Characteristics, ProcessCharacteristic{Descriptions: descs, ValueMeasure: vm})
	}

	// TTL1 (treatment type level 1): first Description "TTL1", then the code/text.
	if tr.TypeLevel1 != "" {
		add([]LangText{{Value: "TTL1"}, {Value: tr.TypeLevel1}}, nil)
	}
	if tr.TypeLevel2 != "" {
		add([]LangText{{Value: "TTL2"}, {Value: tr.TypeLevel2}}, nil)
	}
	if tr.Chemical != "" {
		add([]LangText{{Value: "TTCH"}, {Value: tr.Chemical}}, nil)
	}
	if tr.Temperature != nil {
		add([]LangText{{Value: "TTTM"}}, &Measure{UnitCode: tr.Temperature.Unit, Value: tr.Temperature.Value})
	}
	if tr.Concentration != nil {
		add([]LangText{{Value: "TTCO"}}, &Measure{UnitCode: tr.Concentration.Unit, Value: tr.Concentration.Value})
	}
	if tr.AdditionalInfo != "" {
		add([]LangText{{Value: "TTAI"}, {Lang: tr.LanguageID, Value: tr.AdditionalInfo}}, nil)
	}
	// TTFT (full treatment) should always be present for importers that cannot
	// read structured treatments.
	if tr.FullTreatment != "" {
		add([]LangText{{Value: "TTFT"}, {Lang: tr.LanguageID, Value: tr.FullTreatment}}, nil)
	}

	return ap
}
