// Package staticdata exposes large, versioned JSON reference data to the
// frontend by id, sourced through the artifact registry instead of being
// bundled into the SPA build or embedded in this binary.
package staticdata

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/artifact"
)

// Kind is the artifact kind for static JSON reference data. It is kept
// separate from core's generic_template/workflow_definition kinds so
// registering an id here can never make an internal-only artifact reachable
// from the public lookup endpoint.
const Kind artifact.Kind = "static_data"

type loadable struct {
	json.RawMessage
}

func (loadable) Kind() artifact.Kind { return Kind }

// Parse requires the artifact to be a JSON object with a top-level "data"
// array, never a bare array — so a future field (a version tag, a source
// timestamp) can be added to the envelope without becoming a breaking change
// for every consumer.
func (l *loadable) Parse(raw []byte) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("static data artifact must be a JSON object: %w", err)
	}
	if len(envelope.Data) == 0 {
		return fmt.Errorf(`static data artifact must have a top-level "data" field`)
	}
	var probe []json.RawMessage
	if err := json.Unmarshal(envelope.Data, &probe); err != nil {
		return fmt.Errorf(`static data artifact's "data" field must be a JSON array: %w`, err)
	}
	l.RawMessage = raw
	return nil
}

// Load fetches one version of a static data artifact by id. version is
// required: callers pin the exact version they want, since (id, version)
// identifies immutable content that can be cached indefinitely.
func Load(ctx context.Context, reg *artifact.Registry, id, version string) (json.RawMessage, error) {
	w, err := artifact.Get[loadable](ctx, reg, id, version)
	return w.RawMessage, err
}
