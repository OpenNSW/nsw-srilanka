package plugins

import (
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/shared/deepcopy"

	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
)

// OGAFiles hands an OGA the files in what it is sent as file tokens issued to
// it, which it redeems on this deployment's storage route. A stored value
// never leaves for an OGA as it is.
type OGAFiles struct {
	// ClientFor returns the machine client the outbound service serviceID
	// calls back as.
	ClientFor func(serviceID string) (clientID string, ok bool)
	// IssueForClient returns a token that lets the machine client clientID
	// use value. It never expires: the OGA keeps it in its own records.
	IssueForClient func(clientID, value string) (string, error)
}

// tokenize returns a copy of data, the value at the top-level field root of a
// task's data ("" for all of it), in which each file field renderConfig
// declares holds a token for serviceID's client. data itself is not changed.
// A file to send with no client to issue it to is an error, not sent as it is.
func (f OGAFiles) tokenize(renderConfig json.RawMessage, root string, data any, serviceID string) (any, error) {
	paths, err := filefields.Parse(renderConfig)
	if err != nil {
		return nil, err
	}
	var inData []filefields.Path
	for _, path := range paths {
		if rebased, ok := path.Rebase(root); ok {
			inData = append(inData, rebased)
		}
	}
	if len(inData) == 0 {
		return data, nil
	}
	sent := deepcopy.Value(data)
	err = filefields.Replace(sent, inData, func(value string) (string, error) {
		clientID, ok := f.ClientFor(serviceID)
		if !ok {
			return "", fmt.Errorf("service %q maps to no client in the catalog, so it cannot be sent file tokens", serviceID)
		}
		return f.IssueForClient(clientID, value)
	})
	if err != nil {
		return nil, err
	}
	return sent, nil
}
