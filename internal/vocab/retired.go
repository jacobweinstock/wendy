package vocab

import (
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// RetiredTerm is a word or phrase wendy no longer uses for one of its
// concepts.
type RetiredTerm struct {
	// Term matches the retired use. It is case-sensitive, so a pattern can
	// tell a heading ("Runs") from a verb in a sentence.
	Term *regexp.Regexp
	// Use is what to say instead.
	Use string
}

func retire(pattern, use string) RetiredTerm {
	return RetiredTerm{Term: regexp.MustCompile(pattern), Use: use}
}

// Retired are the words and phrases the glossary replaced. "run" is retired
// only as a noun, so its patterns name the phrases that make it one.
var Retired = []RetiredTerm{
	retire(`\b[Ee]nvironments?\b`, "testbed, or kept testbed for one that is still up"),
	retire(`\b[Pp]rofiles?\b`, "plan"),
	retire(`\b[Ss]cenarios?\b`, "test"),
	retire(`\b[Cc]atalog\b`, "Testbeds or Tests"),
	retire(`\bProviders\b|\bproviders? parts?\b`, "Cluster API"),
	retire(`\b[Ww]orkloads?\b`, "workload cluster"),
	retire(`\b[Ee]vidence\b`, "diagnostics"),
	retire(`\b[Cc]ombos?\b`, "combination"),
	retire(`\bTarget\b`, "testbed"),
	retire(`\bGinkgo label filter\b`, "Only tests with labels"),
	retire(`\b(?:[Aa]|[Tt]he|[Tt]his|[Tt]hat|[Ee]ach|[Ee]very|[Oo]ne|[Yy]our|[Ll]ast|[Nn]ext|[Aa]nother|[Nn]ew|[Ss]ame|[Ff]ailed|[Ff]inished|[Pp]aused|[Aa]ctive|[Pp]revious|[Rr]ecent) runs?\b`, "execution"),
	retire(`\b(?:Cancel|Open|Delete|Start|Stop) runs?\b`, "the same with execution, or Run for Start"),
	retire(`\b[Rr]e-?runs?\b`, "Run again"),
	retire(`\bruns? (?:IDs?|directory|directories)\b`, "execution"),
	retire(`(?m)^[ \t]*Runs[ \t]*$`, "Executions"),
	retire(`\bnot run\b|\bnever run\b`, "skipped, or Not run yet"),
	retire(`\bonFailure\b`, "the choice's label, e.g. On failure"),
	retire(`\bcolocated\b`, "On the management cluster"),
	retire(`\bnetboot\b`, "Network boot"),
	retire(`\bisoboot\b`, "ISO boot"),
	retire(`\b[Pp]layground\b`, "say what it is, e.g. the built-in testbed or the health checks"),
}

// allowed are glossary phrases that contain a retired word.
var allowed = []string{"environment variable", "Environment variable", "workload cluster", "Workload cluster"}

// Finding is a retired term found in a text.
type Finding struct {
	// Term is the text that matched.
	Term string
	// Use is what to say instead.
	Use string
	// Context is the text around the match.
	Context string
}

// Check finds the retired terms in text, in the order they appear. It looks
// at words only, so a caller passes the text people read, not identifiers
// shown as code (a CUE source view, a label such as netboot).
func Check(text string) []Finding {
	// Blanking an allowed phrase keeps every byte offset, so matches still
	// index the original text.
	masked := text
	for _, a := range allowed {
		masked = strings.ReplaceAll(masked, a, strings.Repeat(" ", len(a)))
	}
	type hit struct {
		start, end int
		use        string
	}
	var hits []hit
	for _, r := range Retired {
		for _, m := range r.Term.FindAllStringIndex(masked, -1) {
			hits = append(hits, hit{m[0], m[1], r.Use})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return a.start - b.start })
	out := make([]Finding, 0, len(hits))
	for _, h := range hits {
		out = append(out, Finding{Term: text[h.start:h.end], Use: h.use, Context: around(text, h.start, h.end)})
	}
	return out
}

// around is up to 30 bytes of text on each side of text[start:end], cut at
// rune boundaries.
func around(text string, start, end int) string {
	const pad = 30
	s, e := max(start-pad, 0), min(end+pad, len(text))
	for s > 0 && !utf8.RuneStart(text[s]) {
		s--
	}
	for e < len(text) && !utf8.RuneStart(text[e]) {
		e++
	}
	return strings.TrimSpace(text[s:e])
}
