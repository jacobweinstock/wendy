package build

import "testing"

func TestVersion(t *testing.T) {
	tests := map[string]struct{ raw, want string }{
		"no base":                  {"v0.0.0-20230405123456-abcdef123456", "v0.0.0-gabcdef12"},
		"no base dirty":            {"v0.0.0-20230405123456-abcdef123456+dirty", "v0.0.0-gabcdef12+dirty"},
		"pre-release":              {"v1.2.3-pre.0.20230405123456-abcdef123456", "v1.2.3-pre-gabcdef12"},
		"release":                  {"v1.2.3-0.20230405123456-abcdef123456", "v1.2.3-gabcdef12"},
		"all-digit hash":           {"v0.25.1-0.20261112190000-044057920000", "v0.25.1-g04405792"},
		"tagged release unchanged": {"v1.2.3", "v1.2.3"},
		"devel unchanged":          {"(devel)", "(devel)"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Version(tt.raw); got != tt.want {
				t.Errorf("Version(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
