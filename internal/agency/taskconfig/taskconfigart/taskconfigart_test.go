package taskconfigart_test

import (
	"context"
	"errors"
	"testing"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/testutil"

	"github.com/OpenNSW/nsw-srilanka/internal/agency/taskconfig/taskconfigart"
)

func TestLoad(t *testing.T) {
	reg := artifact.NewRegistry(testutil.MemLoader{
		"ok.json":          []byte(`{"schemaVersion":2,"taskCode":"ok","workflow":"wf_v1","meta":{"title":"Review"}}`),
		"no_workflow.json": []byte(`{"schemaVersion":2,"taskCode":"no_workflow","meta":{"title":"Review"}}`),
		"bad_version.json": []byte(`{"schemaVersion":1,"taskCode":"bad_version","workflow":"wf_v1"}`),
	})
	for _, id := range []string{"ok", "no_workflow", "bad_version"} {
		reg.RegisterArtifact(id, taskconfigart.Kind, "", id+".json")
	}
	ctx := context.Background()

	t.Run("valid config", func(t *testing.T) {
		cfg, err := taskconfigart.Load(ctx, reg, "ok")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Workflow != "wf_v1" || cfg.Meta.Title != "Review" {
			t.Fatalf("cfg = %+v", cfg)
		}
	})

	t.Run("unknown task code is ErrNotFound", func(t *testing.T) {
		if _, err := taskconfigart.Load(ctx, reg, "missing"); !errors.Is(err, artifact.ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("invalid configs are rejected", func(t *testing.T) {
		for _, id := range []string{"no_workflow", "bad_version"} {
			_, err := taskconfigart.Load(ctx, reg, id)
			if err == nil || errors.Is(err, artifact.ErrNotFound) {
				t.Errorf("%s: err = %v, want a validation error", id, err)
			}
		}
	})
}
