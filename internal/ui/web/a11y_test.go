package web

import (
	"regexp"
	"strings"
	"testing"
)

var (
	controlTag = regexp.MustCompile(`(?is)<(/?)(label|input|select|textarea)\b([^>]*)>`)
	attrFor    = regexp.MustCompile(`(?i)\bfor="([^"]*)"`)
	attrID     = regexp.MustCompile(`(?i)\bid="([^"]*)"`)
	attrType   = regexp.MustCompile(`(?i)\btype="([^"]*)"`)
	attrName   = regexp.MustCompile(`(?i)\bname="([^"]*)"`)
)

// unlabelled returns the form controls in page that nothing names: not
// wrapped in a label, not the target of a label's for, and with no
// aria-label or aria-labelledby. Hidden inputs and buttons are not controls a
// person reads.
func unlabelled(page string) []string {
	matches := controlTag.FindAllStringSubmatch(page, -1)

	// Labels may come after their control, so collect every target first.
	targets := map[string]bool{}
	for _, m := range matches {
		if strings.EqualFold(m[2], "label") && m[1] == "" {
			if f := attrFor.FindStringSubmatch(m[3]); f != nil {
				targets[f[1]] = true
			}
		}
	}

	var bad []string
	inLabel := false
	for _, m := range matches {
		closing, tag, attrs := m[1] == "/", strings.ToLower(m[2]), m[3]
		if tag == "label" {
			inLabel = !closing
			continue
		}
		if closing {
			continue
		}
		if t := attrType.FindStringSubmatch(attrs); t != nil {
			switch strings.ToLower(t[1]) {
			case "hidden", "submit", "button":
				continue
			}
		}
		if inLabel || strings.Contains(attrs, "aria-label=") || strings.Contains(attrs, "aria-labelledby=") {
			continue
		}
		if id := attrID.FindStringSubmatch(attrs); id != nil && targets[id[1]] {
			continue
		}
		name := "(no name)"
		if n := attrName.FindStringSubmatch(attrs); n != nil {
			name = n[1]
		}
		bad = append(bad, tag+" "+name)
	}
	return bad
}

func TestEveryFormControlHasALabel(t *testing.T) {
	s := partsServer(t)
	for _, p := range []string{
		"/edit/tests", "/edit/tests?from=smoke", "/edit/suites", "/edit/suites?from=smoke", "/edit-step?i=1",
		"/edit/testbeds", "/edit/testbeds?from=big", "/edit/testbeds?from=adhoc", "/edit/testbeds?from=m",
		"/edit/tinkerbell", "/edit/cluster-api", "/edit/management-cluster", "/edit/workload-cluster", "/edit/hardware", "/edit/hardware?from=ha",
		"/edit/part-dialog?kind=hardware",
		"/run", "/run/quickstart", "/run?testbed=m",
		"/suites/tests",
	} {
		code, page := get(t, s, p)
		if code != 200 {
			t.Errorf("GET %s = %d", p, code)
			continue
		}
		if bad := unlabelled(page); len(bad) > 0 {
			t.Errorf("%s has controls with no label: %v", p, bad)
		}
	}
}

// The checker has to be able to fail, or the test above proves nothing.
func TestLabelCheckerCatchesWhatItShould(t *testing.T) {
	cases := map[string]struct {
		html string
		want int
	}{
		"bare input":          {`<input name="a">`, 1},
		"label for":           {`<label for="x">X</label><input id="x" name="a">`, 0},
		"label after":         {`<input id="x" name="a"><label for="x">X</label>`, 0},
		"wrapped":             {`<label><input name="a"> X</label>`, 0},
		"aria":                {`<select name="a" aria-label="X"></select>`, 0},
		"label without for":   {`<label class="k">X</label><input id="x" name="a">`, 1},
		"hidden and buttons":  {`<input type="hidden" name="a"><input type="submit"><input type="button">`, 0},
		"wrapper then bare":   {`<label><input name="a"></label><textarea name="b"></textarea>`, 1},
		"unrelated label for": {`<label for="y">Y</label><input id="x" name="a">`, 1},
	}
	for name, tc := range cases {
		if got := len(unlabelled(tc.html)); got != tc.want {
			t.Errorf("%s: %d unlabelled, want %d", name, got, tc.want)
		}
	}
}
