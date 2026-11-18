package report

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/tinkerbell/wendy/internal/event"
)

func TestJUnit(t *testing.T) {
	evs := []event.Event{
		{Execution: "1", Type: event.StepFinished, Path: []string{"build", "build", "tinkerbell"}, Status: event.Passed, Ms: 1000},
		{Execution: "1", Combination: "c", Type: event.StepFinished, Path: []string{"smoke", "deploy", "capt.cluster.create"}, Status: event.Failed, Msg: "first"},
		{Execution: "1", Combination: "c", Type: event.StepFinished, Path: []string{"smoke", "deploy", "capt.cluster.create"}, Status: event.Passed, Ms: 2000},
		{Execution: "1", Combination: "c", Type: event.StepFinished, Path: []string{"smoke", "deploy"}, Status: event.Passed},
		{Execution: "1", Combination: "c", Type: event.StepFinished, Path: []string{"smoke", "smoke", "assert.pods"}, Status: event.Errored, Msg: "host broke\ndetail"},
		{Execution: "1", Combination: "c", Type: event.Case, Path: []string{"smoke", "acme", "chainsaw", "policy-a"}, Status: event.Failed, Msg: "denied"},
	}
	var b bytes.Buffer
	if err := JUnit(&b, evs); err != nil {
		t.Fatal(err)
	}
	var got testsuites
	if err := xml.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatalf("%v\n%s", err, b.String())
	}
	if got.Tests != 4 || got.Failures != 1 || got.Errors != 1 || len(got.Suites) != 2 {
		t.Fatalf("totals = %+v\n%s", got, b.String())
	}
	if got.Name != "1" || got.Suites[1].Name != "c" {
		t.Errorf("names = %q, %q; want the execution, then a suite per combination", got.Name, got.Suites[1].Name)
	}
	c := got.Suites[1]
	if c.Cases[0].Failure != nil || c.Cases[0].Time != 2 {
		t.Errorf("retried step should report its last verdict: %+v", c.Cases[0])
	}
	if c.Cases[1].Error == nil || c.Cases[1].Error.Message != "host broke" {
		t.Errorf("errored step = %+v", c.Cases[1])
	}
	if c.Cases[2].Name != "chainsaw › policy-a" || c.Cases[2].Failure == nil {
		t.Errorf("sub-result = %+v", c.Cases[2])
	}
}
