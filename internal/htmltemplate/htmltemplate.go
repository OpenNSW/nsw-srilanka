// Package htmltemplate registers htmlgen templates as the "html_template" kind
// of the core artifact registry.
package htmltemplate

import (
	"context"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/htmlgen"
)

// Kind is the manifest kind of an htmlgen template: the target HTML with
// html/template actions in it, stored as the file itself rather than wrapped
// in JSON.
const Kind artifact.Kind = "html_template"

type loadable struct {
	src []byte
}

func (loadable) Kind() artifact.Kind { return Kind }

// Parse validates raw as an htmlgen template that calls no resolvers.
func (l *loadable) Parse(raw []byte) error {
	if err := htmlgen.Validate(raw); err != nil {
		return err
	}
	l.src = raw
	return nil
}

// Load returns the source of the latest version of the template id. An
// unknown id returns an error matching artifact.ErrNotFound.
func Load(ctx context.Context, reg *artifact.Registry, id string) ([]byte, error) {
	l, err := artifact.Latest[loadable](ctx, reg, id)
	if err != nil {
		return nil, err
	}
	return l.src, nil
}
