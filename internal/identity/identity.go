// Package identity describes the source metricsd measures, so a central
// server can tell each machine and container apart.
package identity

import (
	"context"
	"os"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/host"

	"github.com/brian-nunez/metricsd/internal/environment"
	"github.com/brian-nunez/metricsd/internal/version"
	"github.com/brian-nunez/metricsd/internal/wire"
)

const serviceAccountDir = "/var/run/secrets/kubernetes.io/serviceaccount"

var (
	// Docker (rootful and rootless) mounts /etc/hostname and friends from
	// .../docker/containers/<id>/; Podman from .../overlay-containers/<id>/.
	runtimeIDPattern = regexp.MustCompile(`/(?:docker/containers|overlay-containers)/([0-9a-f]{64})/`)
	// With a host cgroup namespace the cgroup path names the container,
	// e.g. docker-<id>.scope, libpod-<id>.scope, cri-containerd-<id>.scope.
	cgroupIDPattern = regexp.MustCompile(`[0-9a-f]{64}`)
	// Kubelet mounts the termination log from .../pods/<uid>/containers/<name>/.
	kubeContainerPattern = regexp.MustCompile(`/pods/([0-9a-f-]{36})/containers/([^/\s]+)/`)
	kubePodPattern       = regexp.MustCompile(`/pods/([0-9a-f-]{36})/`)
)

// Gather builds the identity for the given environment. Tags from the
// config are attached as-is.
func Gather(ctx context.Context, kind environment.Kind, tags map[string]string) wire.Info {
	info := wire.Info{
		Environment:  string(kind),
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: version.String(),
		StartedAt:    time.Now().Unix(),
		Tags:         tags,
	}
	info.Hostname, _ = os.Hostname()

	// Inside a container this describes the container image's OS, which
	// is the environment being measured. The kernel is always the host's.
	if hi, err := host.InfoWithContext(ctx); err == nil {
		info.Platform = hi.Platform
		info.PlatformVersion = hi.PlatformVersion
		info.KernelVersion = hi.KernelVersion
	}
	if cpus, err := cpu.InfoWithContext(ctx); err == nil && len(cpus) > 0 {
		info.CPUModel = cpus[0].ModelName
	}
	if kind == environment.VM {
		info.Hypervisor, _ = environment.Hypervisor()
	}

	switch {
	case kind == environment.Kubernetes:
		info.Kubernetes = kubernetesInfo(info.Hostname)
		// A private cgroup namespace usually hides the CRI container ID;
		// report the container block only when it was found.
		id := containerID()
		if id != "" {
			info.Container = &wire.ContainerInfo{ID: id}
		}
		info.ID = firstNonEmpty(kubernetesID(info.Kubernetes), id, info.Hostname)
	case kind.IsContainer():
		info.Container = &wire.ContainerInfo{Runtime: string(kind), ID: containerID()}
		info.ID = firstNonEmpty(info.Container.ID, info.Hostname)
	default:
		// /etc/machine-id, which is unique per installation.
		id, _ := host.HostIDWithContext(ctx)
		info.ID = firstNonEmpty(id, info.Hostname)
	}
	return info
}

func containerID() string {
	if data, err := os.ReadFile("/proc/self/cgroup"); err == nil {
		if m := cgroupIDPattern.Find(data); m != nil {
			return string(m)
		}
	}
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		if m := runtimeIDPattern.FindSubmatch(data); m != nil {
			return string(m[1])
		}
	}
	return ""
}

func kubernetesInfo(hostname string) *wire.KubernetesInfo {
	k := &wire.KubernetesInfo{
		Namespace: firstNonEmpty(readTrimmed(serviceAccountDir+"/namespace"), os.Getenv("POD_NAMESPACE")),
		// The pod hostname is the pod name unless spec.hostname overrides it;
		// POD_NAME from the downward API is authoritative when set.
		Pod:  firstNonEmpty(os.Getenv("POD_NAME"), hostname),
		Node: os.Getenv("NODE_NAME"),
	}
	if data, err := os.ReadFile("/proc/self/mountinfo"); err == nil {
		if m := kubeContainerPattern.FindSubmatch(data); m != nil {
			k.PodUID, k.Container = string(m[1]), string(m[2])
		} else if m := kubePodPattern.FindSubmatch(data); m != nil {
			k.PodUID = string(m[1])
		}
	}
	return k
}

// kubernetesID is stable for the life of a pod and unique per container
// in it, which a private cgroup namespace's missing container ID is not.
func kubernetesID(k *wire.KubernetesInfo) string {
	if k.PodUID == "" {
		return ""
	}
	if k.Container == "" {
		return k.PodUID
	}
	return k.PodUID + "/" + k.Container
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func readTrimmed(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
