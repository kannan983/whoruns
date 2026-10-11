package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// drmClient is one GPU client as the Linux kernel reports it in
// /proc/<pid>/fdinfo (i915, xe, amdgpu, msm, panfrost, ...).
type drmClient struct {
	pid      int32
	engines  map[string]uint64  // busy time per engine, ns
	capacity map[string]float64 // parallel units per engine (default 1)
	mem      uint64             // resident bytes
}

var hasNvidia = func() bool { _, err := exec.LookPath("nvidia-smi"); return err == nil }()

// gpuAvailable reports whether GPU numbers can be read on this machine.
func gpuAvailable() bool {
	if hasNvidia {
		return true
	}
	_, err := os.Stat("/dev/dri")
	return runtime.GOOS == "linux" && err == nil
}

// gpuClients adds pid's GPU clients to out, keyed by device + client id. A
// client shared by several processes (after fork) counts once, for the first
// pid that shows it. Only fds pointing at /dev/dri are read, which keeps
// processes with thousands of open files cheap.
func gpuClients(pid int32, out map[string]*drmClient) {
	dir := fmt.Sprintf("/proc/%d/fd", pid)
	fds, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, fd := range fds {
		if link, err := os.Readlink(dir + "/" + fd.Name()); err != nil || !strings.HasPrefix(link, "/dev/dri/") {
			continue
		}
		b, err := os.ReadFile(fmt.Sprintf("/proc/%d/fdinfo/%s", pid, fd.Name()))
		if err != nil {
			continue
		}
		key, c := parseDRM(b)
		if key == "" || out[key] != nil {
			continue
		}
		c.pid = pid
		out[key] = c
	}
}

func parseDRM(b []byte) (string, *drmClient) {
	c := &drmClient{engines: map[string]uint64{}, capacity: map[string]float64{}}
	var pdev, id string
	var resident, legacy uint64 // drm-resident-* (current) or drm-memory-* (older amdgpu)
	for _, line := range bytes.Split(b, []byte("\n")) {
		k, v, ok := strings.Cut(string(line), ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) == 0 {
			continue
		}
		n, _ := strconv.ParseUint(f[0], 10, 64)
		switch {
		case k == "drm-pdev":
			pdev = f[0]
		case k == "drm-client-id":
			id = f[0]
		case strings.HasPrefix(k, "drm-engine-capacity-"):
			c.capacity[strings.TrimPrefix(k, "drm-engine-capacity-")] = float64(n)
		case strings.HasPrefix(k, "drm-engine-"):
			c.engines[strings.TrimPrefix(k, "drm-engine-")] = n
		case strings.HasPrefix(k, "drm-resident-"):
			resident += n * unit(f)
		case strings.HasPrefix(k, "drm-memory-"):
			legacy += n * unit(f)
		}
	}
	if id == "" {
		return "", nil
	}
	c.mem = resident
	if c.mem == 0 {
		c.mem = legacy
	}
	return pdev + "/" + id, c
}

func unit(f []string) uint64 {
	if len(f) < 2 {
		return 1
	}
	switch f[1] {
	case "KiB":
		return 1 << 10
	case "MiB":
		return 1 << 20
	case "GiB":
		return 1 << 30
	}
	return 1
}

// gpuBusy returns each engine's busy % for one client between two samples.
func gpuBusy(now, prev *drmClient, elapsedNs float64) map[string]float64 {
	out := map[string]float64{}
	if prev == nil || elapsedNs <= 0 {
		return out
	}
	for eng, ns := range now.engines {
		before, ok := prev.engines[eng]
		if !ok || ns < before {
			continue
		}
		units := now.capacity[eng]
		if units == 0 {
			units = 1
		}
		out[eng] = float64(ns-before) / elapsedNs / units * 100
	}
	return out
}

type nvidiaProc struct {
	sm  float64 // % of the GPU's compute
	mem uint64  // framebuffer bytes
}

// nvidiaPmon reads per-process use from nvidia-smi. NVIDIA's proprietary
// driver does not fill DRM fdinfo, so this is its only source.
// ponytail: untested on real NVIDIA hardware; columns are found by header name.
func nvidiaPmon() map[int32]nvidiaProc {
	if !hasNvidia {
		return nil
	}
	out, err := exec.Command("nvidia-smi", "pmon", "-c", "1", "-s", "um").Output()
	if err != nil {
		return nil
	}
	return parsePmon(string(out))
}

func parsePmon(out string) map[int32]nvidiaProc {
	res := map[int32]nvidiaProc{}
	col := map[string]int{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		if f[0] == "#" {
			if len(col) == 0 { // first header line names the columns
				for i, name := range f[1:] {
					col[name] = i
				}
			}
			continue
		}
		pi, ok1 := col["pid"]
		si, ok2 := col["sm"]
		if !ok1 || !ok2 || pi >= len(f) || si >= len(f) {
			continue
		}
		pid, err := strconv.Atoi(f[pi])
		if err != nil {
			continue
		}
		p := res[int32(pid)]
		sm, _ := strconv.ParseFloat(f[si], 64) // "-" parses as 0
		p.sm += sm
		if fi, ok := col["fb"]; ok && fi < len(f) {
			mb, _ := strconv.ParseUint(f[fi], 10, 64)
			p.mem += mb << 20
		}
		res[int32(pid)] = p
	}
	return res
}
