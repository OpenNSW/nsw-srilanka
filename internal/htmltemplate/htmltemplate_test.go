package htmltemplate

import (
	"context"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/adapter/generictemplate"
	"github.com/OpenNSW/core/artifact/testutil"
	"github.com/OpenNSW/core/htmlgen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const permitTemplate = `<p>Permit No: {{ .refid.reference_id }}</p>`

// TestLoad loads templates through a real artifact registry, as a manifest
// row of kind html_template registers them.
func TestLoad(t *testing.T) {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"npqs/permit.gohtml": []byte(permitTemplate),
		"npqs/broken.gohtml": []byte(`<p>{{ .a </p>`),
		"npqs/unsafe.gohtml": []byte(`<a href="{{ .u }}`),
	})
	reg.RegisterArtifact("permit", Kind, "", "npqs/permit.gohtml")
	reg.RegisterArtifact("broken", Kind, "", "npqs/broken.gohtml")
	reg.RegisterArtifact("unsafe", Kind, "", "npqs/unsafe.gohtml")
	reg.RegisterArtifact("wrong-kind", generictemplate.Kind, "", "npqs/permit.gohtml")
	ctx := context.Background()

	src, err := Load(ctx, reg, "permit")
	require.NoError(t, err)
	assert.Equal(t, permitTemplate, string(src))

	_, err = Load(ctx, reg, "broken")
	assert.ErrorIs(t, err, htmlgen.ErrParseTemplate, "a template is validated as it is loaded")

	_, err = Load(ctx, reg, "unsafe")
	assert.ErrorIs(t, err, htmlgen.ErrUnsafeTemplate, "a template is validated as it is loaded")

	_, err = Load(ctx, reg, "missing")
	assert.ErrorIs(t, err, artifact.ErrNotFound)

	_, err = Load(ctx, reg, "wrong-kind")
	assert.ErrorIs(t, err, artifact.ErrNotFound, "only a row of kind html_template is a template")
}
