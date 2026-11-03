package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/OpenNSW/core/taskflow/store"
	"github.com/stretchr/testify/assert"
)

func TestTitle(t *testing.T) {
	record := store.TaskRecord{TaskID: "t-1", TaskTemplateID: "trade-export"}
	renders := func(rc string, err error) RenderConfigOf {
		return func(context.Context, store.TaskRecord) (json.RawMessage, error) { return json.RawMessage(rc), err }
	}
	for name, tc := range map[string]struct {
		renderConfigOf RenderConfigOf
		want           string
	}{
		"root title":                    {renders(`{"title":"My Root Title"}`, nil), "My Root Title"},
		"empty title":                   {renders(`{"title":""}`, nil), "trade-export"},
		"no title":                      {renders(`{}`, nil), "trade-export"},
		"null render config":            {renders(`null`, nil), "trade-export"},
		"no render config":              {renders(``, nil), "trade-export"},
		"not JSON":                      {renders(`invalid`, nil), "trade-export"},
		"render config doesn't load":    {renders(``, errors.New("not in the manifest")), "trade-export"},
		"no way to load render configs": {nil, "trade-export"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, Title(context.Background(), tc.renderConfigOf, record))
		})
	}
}
