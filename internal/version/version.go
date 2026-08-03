// Package version surfaces build provenance embedded by the Go toolchain
// (NFR14). No ldflags required: any build from a git checkout carries VCS
// stamps.
package version

import (
	"fmt"
	"runtime"
	"runtime/debug"
)

type Info struct {
	Revision  string `json:"revision"`
	Dirty     bool   `json:"dirty"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

func Get() Info {
	info := Info{Revision: "unknown", BuildTime: "unknown", GoVersion: runtime.Version()}
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

func (i Info) String() string {
	rev := i.Revision
	if len(rev) > 12 {
		rev = rev[:12]
	}
	dirty := ""
	if i.Dirty {
		dirty = " (dirty)"
	}
	return fmt.Sprintf("agentbox %s%s built %s with %s", rev, dirty, i.BuildTime, i.GoVersion)
}
