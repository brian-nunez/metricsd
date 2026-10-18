package version

import "runtime/debug"

// String reports the module version metricsd was built from: the tag for
// `go install ...@vX.Y.Z`, a VCS pseudo-version for builds from a git
// checkout, or "dev".
func String() string {
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
