package config

import (
	"fmt"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"
)

// Renderer evaluates #Render: the manifests and configs wendy derives from a
// Testbed and what the host told it at run time.
type Renderer struct {
	mu     sync.Mutex
	ctx    *cue.Context
	render cue.Value
}

// NewRenderer loads the embedded schema for rendering.
func NewRenderer(builtins []Builtin) (*Renderer, error) {
	ctx := cuecontext.New()
	schema, err := loadSchema(ctx, builtins)
	if err != nil {
		return nil, fmt.Errorf("loading the wendy schema: %w", err)
	}
	return &Renderer{ctx: ctx, render: schema.LookupPath(cue.ParsePath("#Render"))}, nil
}

// Render decodes field of #Render into out. fill sets further top-level
// fields of #Render, such as kubernetesVersion.
func (r *Renderer) Render(tb Testbed, rt any, field string, out any, fill map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	v := r.render.FillPath(cue.ParsePath("testbed"), r.ctx.Encode(tb)).FillPath(cue.ParsePath("rt"), r.ctx.Encode(rt))
	for k, fv := range fill {
		v = v.FillPath(cue.ParsePath(k), r.ctx.Encode(fv))
	}
	fv := v.LookupPath(cue.ParsePath(field))
	if err := fv.Validate(cue.Concrete(true)); err != nil {
		return fmt.Errorf("rendering %s: %s", field, cueerrors.Details(err, nil))
	}
	return fv.Decode(out)
}
