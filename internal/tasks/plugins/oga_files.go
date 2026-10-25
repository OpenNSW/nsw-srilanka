package plugins

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/OpenNSW/core/shared/deepcopy"
	"go.temporal.io/sdk/temporal"

	"github.com/OpenNSW/nsw-srilanka/internal/storage/fileaccess"
	"github.com/OpenNSW/nsw-srilanka/internal/tasks/filefields"
)

// OGAFiles hands an OGA the files in what it is sent as file tokens issued to
// it, which it redeems on this deployment's storage route. A stored value
// never leaves for an OGA as it is: a file another service owns is wrapped in a
// token of this deployment's, never forwarded.
//
// The zero OGAFiles sends values as they are, and is for tests.
type OGAFiles struct {
	// ClientFor returns the machine client the outbound service serviceID
	// calls back as.
	ClientFor func(serviceID string) (clientID string, ok bool)
	// IssueForClient returns a token that lets the machine client clientID
	// open value, and nothing more. It never expires: the OGA keeps it in its
	// own records.
	IssueForClient func(clientID, value string) (string, error)
}

// payloadInput is the input a step sends as its request when it has one.
const payloadInput = "payload"

// tokenizeInputs returns what a step sends serviceID with its declared files as
// tokens for that service's client: the step's payload input when it has one,
// otherwise all its inputs, since those are what is sent. Other inputs are for
// the step's own page, so none of their files get tokens. The inputs are not
// changed; the result is a copy in which payload is replaced, or the inputs
// themselves when no file is declared.
func (f OGAFiles) tokenizeInputs(renderConfig json.RawMessage, inputs map[string]any, serviceID string) (map[string]any, error) {
	root, data := "", any(inputs)
	if payload, ok := inputs[payloadInput]; ok {
		root, data = payloadInput, payload
	}
	sent, err := f.tokenize(renderConfig, root, data, serviceID)
	if err != nil {
		return nil, err
	}
	if root == "" {
		out, _ := sent.(map[string]any)
		return out, nil
	}
	out := make(map[string]any, len(inputs))
	for k, v := range inputs {
		out[k] = v
	}
	out[payloadInput] = sent
	return out, nil
}

// nonRetryable marks a tokenizing failure that no retry can cure: a value whose
// token would be longer than the service accepts stays that long.
func nonRetryable(err error) error {
	if errors.Is(err, fileaccess.ErrTooLong) {
		return temporal.NewNonRetryableApplicationError(err.Error(), "FileTokenTooLong", err)
	}
	return err
}

// tokenize returns a copy of data, the value at the top-level field root of a
// task's data ("" for all of it), in which each file field renderConfig
// declares holds a token for serviceID's client. data itself is not changed.
// A file to send with no client to issue it to is an error, not sent as it is.
func (f OGAFiles) tokenize(renderConfig json.RawMessage, root string, data any, serviceID string) (any, error) {
	if f.IssueForClient == nil {
		return data, nil
	}
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
