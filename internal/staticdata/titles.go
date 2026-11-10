package staticdata

import (
	"context"
	"fmt"

	"github.com/OpenNSW/core/artifact"
)

// Titles names the rows of static_data artifacts by their const, for documents
// the server renders, such as a port's name for its UN/LOCODE. Each call reads
// the artifact through the registry, so an edited artifact is seen on the next
// render, as everywhere else.
type Titles struct {
	artifacts *artifact.Set
}

// NewTitles returns a Titles reading from artifacts.
func NewTitles(artifacts *artifact.Set) *Titles {
	if artifacts == nil {
		panic("artifacts is nil")
	}
	return &Titles{artifacts: artifacts}
}

// Of returns the title of every row of one version of a static_data artifact,
// keyed by const. Where two rows share a const, the first one's title is kept.
// An unknown (id, version) returns an error matching artifact.ErrNotFound, and
// a row without a const or a title one matching artifact.ErrInvalid.
func (t *Titles) Of(ctx context.Context, id, version string) (map[string]string, error) {
	raw, err := Load(ctx, t.artifacts, id, version)
	if err != nil {
		return nil, err
	}
	options, err := parseOptions(raw)
	if err != nil {
		return nil, fmt.Errorf("static data %q version %q: %w: %w", id, version, artifact.ErrInvalid, err)
	}
	titles := make(map[string]string, len(options))
	for _, opt := range options {
		if _, seen := titles[opt.Const]; !seen {
			titles[opt.Const] = opt.Title
		}
	}
	return titles, nil
}
