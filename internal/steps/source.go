package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/engine"
	"github.com/tinkerbell/wendy/internal/provider/registry"
	"github.com/tinkerbell/wendy/internal/source"
)

func init() {
	engine.Register("source.tinkerbell", "Builds the tinkerbell and tink-agent images and the chart from a checkout, and publishes the images.",
		"{repo?: string, ref?: string}", func(ctx context.Context, s *engine.Step, p config.Source) error {
			b, err := builder(ctx, s)
			if err != nil {
				return err
			}
			co, err := resolve(ctx, s, b, "tinkerbell", p)
			if err != nil {
				return err
			}
			out, err := b.BuildTinkerbell(ctx, co)
			if err != nil {
				return err
			}
			return outputs(s, out)
		})
	engine.Register("source.capt", "Builds the CAPT manager image and a clusterctl release from a checkout, and publishes the image.",
		"{repo?: string, ref?: string}", func(ctx context.Context, s *engine.Step, p config.Source) error {
			b, err := builder(ctx, s)
			if err != nil {
				return err
			}
			co, err := resolve(ctx, s, b, "capt", p)
			if err != nil {
				return err
			}
			out, err := b.BuildCAPT(ctx, co, "")
			if err != nil {
				return err
			}
			return outputs(s, out)
		})
}

func builder(ctx context.Context, s *engine.Step) (source.Builder, error) {
	if err := registry.Ensure(ctx, logf(s)); err != nil {
		return source.Builder{}, err
	}
	if err := registry.EnsureBuilder(ctx); err != nil {
		return source.Builder{}, err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return source.Builder{}, err
	}
	return source.Builder{
		Cache: filepath.Join(cache, "wendy", "src"),
		Run:   source.Exec,
		Log:   engine.LineWriter(func(l string) { s.Log.Info(l) }),
	}, nil
}

func resolve(ctx context.Context, s *engine.Step, b source.Builder, component string, p config.Source) (source.Checkout, error) {
	co, err := b.Resolve(ctx, component, p)
	if err != nil {
		return co, err
	}
	dirty := ""
	if co.Dirty {
		dirty = " (dirty)"
	}
	short := co.Commit
	if len(short) > 7 {
		short = short[:7]
	}
	s.Log.Info(fmt.Sprintf("%s at %s%s", co.Repo, short, dirty))
	return co, nil
}

// outputs flattens a build's result into the step's outputs.
func outputs(s *engine.Step, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &s.Outputs)
}
