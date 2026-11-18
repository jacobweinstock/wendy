package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/tinkerbell/wendy/internal/config"
	"github.com/tinkerbell/wendy/internal/preview"
	"github.com/tinkerbell/wendy/internal/vocab"
)

// partSlugs are the names `wendy list` takes for the kinds of part, in the
// order a testbed names them.
func partSlugs() []string {
	var out []string
	for _, kind := range config.PartKinds {
		k, _ := vocab.KindOf(kind)
		out = append(out, k.Slug)
	}
	return out
}

// partKindOfSlug is the kind of part a `wendy list` name stands for.
func partKindOfSlug(slug string) (string, bool) {
	for _, kind := range config.PartKinds {
		if k, _ := vocab.KindOf(kind); k.Slug == slug {
			return kind, true
		}
	}
	return "", false
}

// listParts writes each part of a kind with where it is defined and the
// testbeds that use it.
func listParts(w io.Writer, cat *config.Catalog, kind string) {
	for _, n := range config.Names(cat.Parts(kind)) {
		used := strings.Join(partUsers(cat, kind, n), ",")
		if used == "" {
			used = "unused"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", n, cat.Parts(kind)[n].Origin, used)
	}
}

// varyingParts are the kinds of part that differ between the combinations, in
// the order a testbed names them. Parts every combination shares say nothing
// about how they differ, so they are left out of a preview.
func varyingParts(combinations []*preview.Combination) []string {
	var out []string
	for _, kind := range config.PartKinds {
		for _, c := range combinations[1:] {
			if c.Parts[kind] != combinations[0].Parts[kind] {
				out = append(out, kind)
				break
			}
		}
	}
	return out
}

// partUsers are the testbeds that use a part, themselves or in an axis of
// their matrix, sorted.
func partUsers(cat *config.Catalog, kind, name string) []string {
	var out []string
	for _, tb := range config.Names(cat.Testbeds) {
		if cat.Testbeds[tb].Value.Picks(kind, name) {
			out = append(out, tb)
		}
	}
	return out
}
