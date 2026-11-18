package ui

import "testing"

func TestNodeLabelNamesPhases(t *testing.T) {
	steps := []*Node{{Name: "management.create"}, {Name: "bmc.start"}}
	for _, c := range []struct {
		node *Node
		want string
	}{
		{&Node{Kind: KindBuild, Name: "Build"}, "Build images"},
		{&Node{Kind: KindSuite, Name: "setup", Children: steps}, "Set up testbed  (2 steps)"},
		{&Node{Kind: KindSuite, Name: "diagnostics"}, "Collect diagnostics"},
		{&Node{Kind: KindSuite, Name: "teardown"}, "Tear down"},
		{&Node{Kind: KindSuite, Name: "smoke"}, "smoke"},
		{&Node{Kind: KindTest, Name: "deploy", Reused: true}, "deploy  (reused)"},
	} {
		if got := c.node.Label(); got != c.want {
			t.Errorf("%s %q: Label() = %q, want %q", c.node.Kind, c.node.Name, got, c.want)
		}
	}
}
