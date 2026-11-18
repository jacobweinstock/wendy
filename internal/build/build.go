// Package build reports the version of the running binary from the VCS
// information the Go toolchain embeds at build time.
package build

import (
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"
)

// pseudoVersion represents a parsed Go module pseudo-version.
type pseudoVersion struct {
	BaseVersion string // vX.Y.Z or vX.Y.Z-pre
	CommitHash  string // Short commit hash
	IsDirty     bool   // Whether version has +dirty suffix
}

// formType represents the three possible forms of pseudo-versions.
type formType int

const (
	noBaseForm formType = iota
	preReleaseForm
	releaseForm
)

// The patterns follow https://go.dev/ref/mod#pseudo-versions.
var patterns = []struct {
	form formType
	re   *regexp.Regexp
}{
	{noBaseForm, regexp.MustCompile(`^v(\d+)\.0\.0-(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`)},
	{preReleaseForm, regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)-pre\.0\.(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`)},
	{releaseForm, regexp.MustCompile(`^v(\d+)\.(\d+)\.(\d+)-0\.(\d{8})(\d{6})-([a-f0-9]{12})(?:\+dirty)?$`)},
}

// GitRevision returns the version of the current build, taken from the VCS
// state Go embeds in the binary. A build with uncommitted changes is suffixed
// with "+dirty". It returns "unknown" when the binary carries no build info.
func GitRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return Version(info.Main.Version)
}

// Version shortens a pseudo-version to its base version and abbreviated commit
// hash. Any other string, such as a tagged release or "(devel)", is returned
// unchanged.
func Version(raw string) string {
	v, err := parse(raw)
	if err != nil {
		return raw
	}
	return v.String()
}

func parse(version string) (*pseudoVersion, error) {
	for _, p := range patterns {
		m := p.re.FindStringSubmatch(version)
		if m == nil {
			continue
		}
		pv := &pseudoVersion{
			IsDirty:    strings.HasSuffix(version, "+dirty"),
			CommitHash: m[len(m)-1][:8], // Same length as a git short hash.
		}
		switch p.form {
		case noBaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.0.0", m[1])
		case preReleaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.%s.%s-pre", m[1], m[2], m[3])
		case releaseForm:
			pv.BaseVersion = fmt.Sprintf("v%s.%s.%s", m[1], m[2], m[3])
		}
		return pv, nil
	}
	return nil, fmt.Errorf("invalid pseudo-version format: %q", version)
}

func (pv *pseudoVersion) String() string {
	// The "g" prefix, as in git describe, keeps an all-digit hash from forming a
	// numeric SemVer identifier, which is invalid with a leading zero.
	base := fmt.Sprintf("%s-g%s", pv.BaseVersion, pv.CommitHash)
	if pv.IsDirty {
		return base + "+dirty"
	}
	return base
}
