// Package report renders execution reports from events.
package report

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinkerbell/wendy/internal/event"
)

type testsuites struct {
	XMLName  xml.Name    `xml:"testsuites"`
	Name     string      `xml:"name,attr"`
	Tests    int         `xml:"tests,attr"`
	Failures int         `xml:"failures,attr"`
	Errors   int         `xml:"errors,attr"`
	Skipped  int         `xml:"skipped,attr"`
	Time     float64     `xml:"time,attr"`
	Suites   []testsuite `xml:"testsuite"`
}

type testsuite struct {
	Name     string     `xml:"name,attr"`
	Tests    int        `xml:"tests,attr"`
	Failures int        `xml:"failures,attr"`
	Errors   int        `xml:"errors,attr"`
	Skipped  int        `xml:"skipped,attr"`
	Time     float64    `xml:"time,attr"`
	Cases    []testcase `xml:"testcase"`
}

type testcase struct {
	Name      string   `xml:"name,attr"`
	Classname string   `xml:"classname,attr"`
	Time      float64  `xml:"time,attr"`
	Failure   *message `xml:"failure,omitempty"`
	Error     *message `xml:"error,omitempty"`
	Skipped   *message `xml:"skipped,omitempty"`
}

type message struct {
	Message string `xml:"message,attr,omitempty"`
	Type    string `xml:"type,attr,omitempty"`
	Body    string `xml:",chardata"`
}

// JUnit writes the JUnit report of an execution's events. Each combination is a
// testsuite; each step, and each sub-result a step reported, is a testcase.
// Product failures are <failure>; harness and host problems are <error>.
func JUnit(w io.Writer, evs []event.Event) error {
	type key struct{ combination, path string }
	var order []key
	last := map[key]event.Event{}
	execution := ""
	for _, e := range evs {
		if execution == "" {
			execution = e.Execution
		}
		isStep := e.Type == event.StepFinished && len(e.Path) == 3
		if !isStep && e.Type != event.Case {
			continue
		}
		k := key{e.Combination, strings.Join(e.Path, "\x00")}
		if _, seen := last[k]; !seen {
			order = append(order, k)
		}
		last[k] = e
	}

	out := testsuites{Name: execution}
	byCombination := map[string]*testsuite{}
	var combinations []string
	for _, k := range order {
		e := last[k]
		name := e.Combination
		if name == "" {
			name = "build"
		}
		ts, ok := byCombination[name]
		if !ok {
			ts = &testsuite{Name: name}
			byCombination[name] = ts
			combinations = append(combinations, name)
		}
		tc := testcase{
			Classname: strings.Join(e.Path[:2], "."),
			Name:      strings.Join(e.Path[2:], " › "),
			Time:      float64(e.Ms) / 1000,
		}
		switch e.Status {
		case event.Failed:
			tc.Failure = &message{Message: firstLine(e.Msg), Type: "failed", Body: e.Msg}
			ts.Failures++
		case event.Errored, event.TimedOut, event.Cancelled:
			tc.Error = &message{Message: firstLine(e.Msg), Type: string(e.Status), Body: e.Msg}
			ts.Errors++
		case event.Skipped:
			tc.Skipped = &message{Message: e.Msg}
			ts.Skipped++
		default:
		}
		ts.Tests++
		ts.Time += tc.Time
		ts.Cases = append(ts.Cases, tc)
	}
	for _, name := range combinations {
		ts := byCombination[name]
		out.Suites = append(out.Suites, *ts)
		out.Tests += ts.Tests
		out.Failures += ts.Failures
		out.Errors += ts.Errors
		out.Skipped += ts.Skipped
		out.Time += ts.Time
	}
	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(out); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

// WriteJUnit writes report.junit.xml into an execution directory from its
// events.
func WriteJUnit(dir string) error {
	evs, err := event.ReadFile(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "report.junit.xml"))
	if err != nil {
		return err
	}
	if err := JUnit(f, evs); err != nil {
		f.Close()
		return fmt.Errorf("writing the JUnit report: %w", err)
	}
	return f.Close()
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
