// Package version surfaces build provenance embedded by the Go toolchain
// (NFR14). Any build from a git checkout carries VCS stamps with no ldflags
// needed - which is the whole identity of a build made from source.
//
// A RELEASE needs one thing the toolchain cannot supply. A tag is a label
// applied to a commit from outside, so a binary cannot discover it by looking
// at itself, and a downloaded release has no checkout to ask. `make dist`
// therefore stamps Tag when it is packaging a tagged version, and only then:
// an empty Tag is what "built from source" means, and is the signal not to
// pester somebody running their own build about upgrading.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

// Tag is set at link time by `make dist` (-X). Empty in every build from
// source, including this repo's own `make build` - see the package comment.
var Tag string

type Info struct {
	Tag       string `json:"tag,omitempty"`
	Revision  string `json:"revision"`
	Dirty     bool   `json:"dirty"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

func Get() Info {
	info := Info{Tag: Tag, Revision: "unknown", BuildTime: "unknown", GoVersion: runtime.Version()}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			info.BuildTime = s.Value
		case "vcs.modified":
			info.Dirty = s.Value == "true"
		}
	}
	return info
}

// Released says whether this build came from a release rather than from
// somebody's checkout. A dirty tree is never a release even if it was stamped,
// because the code no longer matches the tag it claims.
func (i Info) Released() bool { return i.Tag != "" && !i.Dirty }

func (i Info) String() string {
	rev := i.Revision
	if len(rev) > 12 {
		rev = rev[:12]
	}
	dirty := ""
	if i.Dirty {
		dirty = " (dirty)"
	}
	// The tag leads when there is one: it is the thing a person recognises, and
	// the revision stays because it is what identifies the code.
	if i.Tag != "" {
		return fmt.Sprintf("agentbox %s (%s)%s built %s with %s", i.Tag, rev, dirty, i.BuildTime, i.GoVersion)
	}
	return fmt.Sprintf("agentbox %s%s built %s with %s", rev, dirty, i.BuildTime, i.GoVersion)
}
