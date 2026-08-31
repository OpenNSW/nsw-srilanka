// Package consolidation associates the containers SLPA pre-advised for a
// declaration with the containers on the service order raised against it.
//
// SLPA holds the two sides separately: the cap containers a terminal recorded
// against the CUSDEC, and the containers priced on the export service order.
// Consolidation is what ties each pair together, and it must be done before a
// gate pass can be issued for any of them.
package consolidation

import (
	"fmt"
	"sort"
	"strings"
)

// CapContainer is one FCL container SLPA pre-advised against the declaration.
// Only the fields this integration acts on are modelled; the CMS sends more
// (vessel, ISO code, VGM) and it stays in the raw response for the panel.
type CapContainer struct {
	Sqid            string `json:"sqid"`
	CusdecSerial    string `json:"cusdecserial"`
	ContainerNo     string `json:"container_no"`
	ContainerSize   string `json:"container_size"`
	ConStatus       string `json:"con_status"`
	SOContainerSqid any    `json:"so_container_sqid"`
}

// consolidated reports whether the CMS has already paired this container.
//
// The field is null until consolidation and carries the service-order
// container's sqid afterwards, so it is also how a redelivered or repeated run
// recognises work already done.
func (c CapContainer) consolidated() bool {
	if c.SOContainerSqid == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(c.SOContainerSqid)) != ""
}

// SOContainer is one container priced on the export service order.
type SOContainer struct {
	Sqid          string `json:"sqid"`
	ExportSOID    int    `json:"export_so_id"`
	ContainerNo   string `json:"ContainerNumber"`
	ContainerSize string `json:"ContainerSize"`
	Service       int    `json:"Service"`
}

// FetchResponse is the CMS's answer to the consolidation lookup, as it arrives
// nested under the envelope's "data".
type FetchResponse struct {
	CapContainers []CapContainer `json:"cap_containers"`
	SOContainers  []SOContainer  `json:"so_containers"`
}

// Pair is one association to save: a cap container and the service-order
// container that carries the same container number.
type Pair struct {
	ID            string `json:"id"`
	SOContainerID string `json:"so_container_id"`

	// ContainerNo is not part of the request the CMS reads. It is kept so the
	// trader's panel and the gate-pass step that follows can name the container
	// a pair is about, rather than showing an obfuscated sqid.
	ContainerNo string `json:"-"`
}

// SaveRequest is the body of the save call.
type SaveRequest struct {
	Containers []Pair `json:"containers"`
}

// Row is one line of the trader's consolidation form: a container SLPA
// pre-advised, and the service-order container they are pairing it with.
//
// The pairing is the trader's to make, and it cannot be derived. The two sides
// carry different numbers by design: a cap container is the real container the
// terminal pre-advised, while a service-order container is the placeholder the
// order was priced against. Only the trader knows which placeholder a real
// container answers to.
type Row struct {
	CapContainerNo string `json:"cap_container_no"`
	CapSqid        string `json:"cap_sqid"`
	SOContainerNo  string `json:"so_container_no"`
	Consolidate    bool   `json:"consolidate"`
}

// Rows builds the form the trader is shown: one line per pre-advised container
// SLPA has not consolidated yet.
//
// Nothing is matched on the container number, because the two sides never share
// one: the pre-advised number is the real container, the service-order number is
// the placeholder it was priced against. A line is pre-filled only when the
// choice is not a choice at all — one container to pair, one placeholder to pair
// it with. Everything else is left for the trader, empty and unticked, rather
// than filled with a guess they might submit unread.
func Rows(resp FetchResponse) []Row {
	rows := make([]Row, 0, len(resp.CapContainers))
	for _, capContainer := range resp.CapContainers {
		if capContainer.consolidated() || strings.TrimSpace(capContainer.ContainerNo) == "" {
			continue
		}
		rows = append(rows, Row{
			CapContainerNo: strings.TrimSpace(capContainer.ContainerNo),
			CapSqid:        strings.TrimSpace(capContainer.Sqid),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CapContainerNo < rows[j].CapContainerNo })

	if len(rows) == 1 && len(resp.SOContainers) == 1 {
		if only := strings.TrimSpace(resp.SOContainers[0].ContainerNo); only != "" {
			rows[0].SOContainerNo = only
			rows[0].Consolidate = true
		}
	}
	return rows
}

// AlreadyConsolidated lists the containers SLPA has already paired, which are
// not offered again but still count as consolidated for the gate pass.
func AlreadyConsolidated(resp FetchResponse) []string {
	var done []string
	for _, capContainer := range resp.CapContainers {
		if capContainer.consolidated() && strings.TrimSpace(capContainer.ContainerNo) != "" {
			done = append(done, strings.TrimSpace(capContainer.ContainerNo))
		}
	}
	sort.Strings(done)
	return done
}

// SOContainerNumbers lists what the service order priced, for the trader to
// choose from.
func SOContainerNumbers(resp FetchResponse) []string {
	var numbers []string
	for _, so := range resp.SOContainers {
		if no := strings.TrimSpace(so.ContainerNo); no != "" {
			numbers = append(numbers, no)
		}
	}
	sort.Strings(numbers)
	return numbers
}

// Selection is what the trader submitted, resolved against what SLPA offered.
type Selection struct {
	// Pairs are the associations to save, in the order the rows were shown.
	Pairs []Pair

	// Unresolved names a row the trader ticked whose service-order container
	// SLPA does not hold. It is reported rather than dropped silently: the CMS
	// would refuse the sqid we cannot supply, and the trader would have no way
	// to see which line was at fault.
	Unresolved []string
}

// Resolve turns the submitted rows into the pairs the CMS reads.
//
// The service-order container the trader chose is looked up by its number to
// recover the sqid SLPA issued for it: they work in the numbers they are shown,
// and the CMS works in sqids. Only ticked rows are sent, so declining a
// container is a decision this honours rather than one it overrides.
func Resolve(rows []Row, soContainers []SOContainer) Selection {
	soByNo := make(map[string]SOContainer, len(soContainers))
	for _, so := range soContainers {
		if no := normalise(so.ContainerNo); no != "" {
			soByNo[no] = so
		}
	}

	var selection Selection
	for _, row := range rows {
		if !row.Consolidate {
			continue
		}
		capSqid := strings.TrimSpace(row.CapSqid)
		so, ok := soByNo[normalise(row.SOContainerNo)]
		if !ok || strings.TrimSpace(so.Sqid) == "" || capSqid == "" {
			selection.Unresolved = append(selection.Unresolved, row.CapContainerNo)
			continue
		}
		selection.Pairs = append(selection.Pairs, Pair{
			ID:            capSqid,
			SOContainerID: strings.TrimSpace(so.Sqid),
			ContainerNo:   row.CapContainerNo,
		})
	}
	return selection
}

// Containers lists what a selection consolidates.
func (s Selection) Containers() []string {
	out := make([]string, 0, len(s.Pairs))
	for _, p := range s.Pairs {
		out = append(out, p.ContainerNo)
	}
	return out
}

// normalise makes what the trader typed comparable with what SLPA holds, which
// has been seen to differ in case and surrounding space.
func normalise(containerNo string) string {
	return strings.ToUpper(strings.TrimSpace(containerNo))
}
