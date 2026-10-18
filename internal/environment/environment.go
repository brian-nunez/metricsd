// Package environment detects what metricsd is running in: a physical
// machine, a virtual machine, or a container. Each is measured as its own
// machine; a container on a VM on a server is three separate sources.
package environment

import (
	"os"
	"runtime"
	"strings"
	"sync"
)

type Kind string

const (
	Physical   Kind = "physical"
	VM         Kind = "vm"
	Docker     Kind = "docker"
	Podman     Kind = "podman"
	Kubernetes Kind = "kubernetes"
	// Container is any other container runtime (LXC, systemd-nspawn,
	// containerd without Kubernetes) that identifies itself.
	Container Kind = "container"
)

var Kinds = []Kind{Physical, VM, Docker, Podman, Kubernetes, Container}

// IsContainer reports whether metrics come from cgroups rather than the
// machine-wide kernel interfaces.
func (k Kind) IsContainer() bool {
	switch k {
	case Docker, Podman, Kubernetes, Container:
		return true
	}
	return false
}

func Valid(s string) bool {
	for _, k := range Kinds {
		if string(k) == s {
			return true
		}
	}
	return false
}

// Detect returns the environment of the current process. The result is
// computed once.
var Detect = sync.OnceValue(func() Kind {
	if k := detectContainer(); k != "" {
		return k
	}
	if _, ok := Hypervisor(); ok {
		return VM
	}
	return Physical
})

// detectContainer checks the markers each runtime leaves inside its
// containers. They are the same for rootful and rootless runtimes.
// Kubernetes is checked first because pods also run on Docker-compatible
// runtimes that leave their own markers.
func detectContainer() Kind {
	switch {
	case os.Getenv("KUBERNETES_SERVICE_HOST") != "" || exists("/var/run/secrets/kubernetes.io"):
		return Kubernetes
	case exists("/run/.containerenv") || os.Getenv("container") == "podman":
		return Podman
	case exists("/.dockerenv"):
		return Docker
	case os.Getenv("container") != "":
		return Container
	}
	return ""
}

var dmiHypervisors = []struct{ match, name string }{
	{"qemu", "kvm"},
	{"kvm", "kvm"},
	{"vmware", "vmware"},
	{"virtualbox", "virtualbox"},
	{"innotek", "virtualbox"},
	{"xen", "xen"},
	{"microsoft corporation virtual machine", "hyperv"},
	{"amazon ec2", "aws"},
	{"google compute engine", "gce"},
	{"digitalocean", "digitalocean"},
	{"hetzner", "hetzner"},
	{"openstack", "openstack"},
	{"parallels", "parallels"},
	{"bhyve", "bhyve"},
	{"apple virtualization", "apple"},
}

// Hypervisor reports whether the machine is a virtual machine and, when
// recognizable, which hypervisor or cloud runs it. On x86 the CPU's
// hypervisor flag is authoritative, which keeps bare-metal cloud servers
// (e.g. EC2 *.metal) classified as physical despite cloud DMI strings.
func Hypervisor() (string, bool) {
	dmi := strings.ToLower(strings.Join([]string{
		readTrimmed("/sys/class/dmi/id/sys_vendor"),
		readTrimmed("/sys/class/dmi/id/product_name"),
	}, " "))
	name := ""
	for _, h := range dmiHypervisors {
		if strings.Contains(dmi, h.match) {
			name = h.name
			break
		}
	}

	switch runtime.GOARCH {
	case "amd64", "386":
		if !cpuHasHypervisorFlag() {
			return "", false
		}
		if name == "" {
			name = "unknown"
		}
		return name, true
	default:
		return name, name != ""
	}
}

func cpuHasHypervisorFlag() bool {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if key, flags, ok := strings.Cut(line, ":"); ok && strings.TrimSpace(key) == "flags" {
			for _, f := range strings.Fields(flags) {
				if f == "hypervisor" {
					return true
				}
			}
			return false
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
