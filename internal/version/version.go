package version

import "runtime/debug"

// version is set at link time for release builds:
//
//	-ldflags '-X github.com/brian-nunez/metricsd/internal/version.version=v1.2.3'
var version string

// String reports the version metricsd was built as: the link-time version
// for release archives, the module tag for `go install ...@vX.Y.Z`, a VCS
// pseudo-version for builds from a git checkout, or "dev".
func String() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
