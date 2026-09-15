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

// CapContainer is one FCL container SLPA pre-advised against the declaration,
// as their lookup answers. Only the fields this integration acts on are
// modelled; the CMS sends more against each container — vessel, ISO code, VGM,
// line operator — and it stays in the raw response for the panel.
type CapContainer struct {
	Sqid          string `json:"sqid"`
	ContainerNo   string `json:"container_no"`
	ContainerSize string `json:"container_size"`

	// SOContainerID is the service-order container this one has been paired
	// with: null until it is consolidated. Typed any because only its presence
	// is read here, and the CMS has never sent it populated for us to see which
	// JSON type it takes.
	SOContainerID any `json:"so_container_id"`
}

// consolidated reports whether the CMS has already paired this container.
func (c CapContainer) consolidated() bool { return pairedWith(c.SOContainerID) }

// SOContainerIDField is the name the pairing is recorded under on the rows the
// lookup writes to the task record. It is SLPA's own name for it, so a row read
// back off a task says the same thing their answer did; named here because the
// reader of those rows is elsewhere — the trader's form projector — and the two
// must agree.
const SOContainerIDField = "so_container_id"

// Paired reports whether the CMS has already paired the container one recorded
// row describes, for a caller reading those rows back off the task record
// rather than holding the CapContainer they were built from.
//
// It exists so there is one answer to "is this one already done": the projector
// leaves these out of what it offers the trader, and offering an already-paired
// container is a save the CMS refuses.
func Paired(row map[string]any) bool { return pairedWith(row[SOContainerIDField]) }

// pairedWith reports whether the CMS has recorded a pairing in the value given.
//
// Rendered rather than asserted to a string: the field is null when unpaired and
// the CMS has not promised which type it takes when set. A hard assertion would
// read anything unexpected as "not paired yet", which is the answer that does
// damage — it puts a container that is already consolidated back in front of the
// trader.
func pairedWith(soContainerID any) bool {
	if soContainerID == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(soContainerID)) != ""
}

// SOContainer is one container priced on the export service order.
//
// As on CapContainer, only what this integration acts on is modelled. The CMS
// sends more against each container — the order id it belongs to, the service
// priced on it — and modelling those cost the whole lookup once: they were typed
// as int, the CMS sent one of them as a string, and the decode of the entire
// answer failed, leaving the trader a form with no containers to pick from and a
// message saying only that the answer could not be read. A field nothing reads
// is a field that can only break the decode.
type SOContainer struct {
	Sqid          string `json:"sqid"`
	ContainerNo   string `json:"ContainerNumber"`
	ContainerSize string `json:"ContainerSize"`
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

// Row is one container SLPA pre-advised and has not consolidated yet: one of
// the real containers a trader is choosing between.
//
// The pairing itself is not modelled here. It is the trader's to make and it
// cannot be derived — the two sides carry different numbers by design, a cap
// container being the real container the terminal pre-advised and a
// service-order container the placeholder the order was priced against — and
// each branch resolves the one pairing it owns through PairOne.
type Row struct {
	CapContainerNo string `json:"cap_container_no"`
	CapSqid        string `json:"cap_sqid"`
}

// Rows lists the pre-advised containers SLPA has not consolidated yet: what
// there is to do under this declaration, which is what the lookup reports and
// what the trader's panel describes.
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

// normalise makes what the trader typed comparable with what SLPA holds, which
// has been seen to differ in case and surrounding space.
func normalise(containerNo string) string {
	return strings.ToUpper(strings.TrimSpace(containerNo))
}

// CapContainerNumbers lists the real containers available to pair: the ones the
// terminal has pre-advised against this declaration and SLPA has not already
// consolidated.
//
// This is the list the trader picks from. A container missing from it has not
// been pre-advised in Navis yet — the trader does that there and comes back,
// which is why the branch waits rather than failing.
func CapContainerNumbers(resp FetchResponse) []string {
	var numbers []string
	for _, capContainer := range resp.CapContainers {
		if capContainer.consolidated() {
			continue
		}
		if no := strings.TrimSpace(capContainer.ContainerNo); no != "" {
			numbers = append(numbers, no)
		}
	}
	sort.Strings(numbers)
	return numbers
}

// PairOne associates one real container with the placeholder a branch owns.
//
// Both sides are named by number, because that is what the trader and the order
// speak in, and both are resolved to the sqids the CMS reads. A number neither
// side holds is reported rather than sent: the CMS would refuse a sqid we could
// not supply, and the trader would have no way to see which half was at fault.
func PairOne(capNo, soNo string, resp FetchResponse) (Pair, error) {
	var pair Pair

	capNorm, soNorm := strings.TrimSpace(capNo), normalise(soNo)
	if capNorm == "" {
		return pair, fmt.Errorf("choose the real container this one is being consolidated against")
	}

	for _, capContainer := range resp.CapContainers {
		// Matched on the sqid alone: it is what the projector offers as each
		// option's value, so it is what the trader's answer carries.
		sqid := strings.TrimSpace(capContainer.Sqid)
		if sqid != "" && sqid == capNorm {
			pair.ID = sqid
			pair.ContainerNo = strings.TrimSpace(capContainer.ContainerNo)
			break
		}
	}
	if pair.ID == "" {
		return Pair{}, fmt.Errorf("SLPA is no longer offering container %s for consolidation", capNo)
	}

	for _, so := range resp.SOContainers {
		if normalise(so.ContainerNo) == soNorm && strings.TrimSpace(so.Sqid) != "" {
			pair.SOContainerID = strings.TrimSpace(so.Sqid)
			break
		}
	}
	if pair.SOContainerID == "" {
		return Pair{}, fmt.Errorf("SLPA does not hold service order container %s", soNo)
	}
	return pair, nil
}
