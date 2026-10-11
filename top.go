package main

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type appRow struct {
	Name   string
	Mem    uint64
	CPU    float64 // % of one core, like top
	Procs  int
	Folder string
	Docker bool
}

type cpuSnap struct {
	at    time.Time
	times map[int32]float64
}

type sysInfo struct {
	used, total uint64
	cpu         float64
}

func snapCPU() cpuSnap {
	s := cpuSnap{time.Now(), map[int32]float64{}}
	pids, _ := process.Pids()
	for _, pid := range pids {
		if p, err := process.NewProcess(pid); err == nil {
			if ct, err := p.Times(); err == nil {
				s.times[pid] = ct.User + ct.System
			}
		}
	}
	return s
}

// sysStats reads machine-wide memory and CPU. With wait it samples CPU over
// one second; without, it measures since the previous call.
func sysStats(wait bool) sysInfo {
	var s sysInfo
	if vm, err := mem.VirtualMemory(); err == nil {
		s.used, s.total = vm.Used, vm.Total
	}
	interval := time.Duration(0)
	if wait {
		interval = time.Second
	}
	if p, err := cpu.Percent(interval, false); err == nil && len(p) > 0 {
		s.cpu = p[0]
	}
	return s
}

// listApps groups processes into apps: a child running the same executable
// as its parent joins the parent's row (so 40 brave processes are one row, but
// node started from two project shells is two rows). CPU is measured against
// prev; pass the returned snapshot next time.
func listApps(prev cpuSnap) ([]appRow, cpuSnap) {
	procs, _ := process.Processes()
	dk := loadDocker()
	now := cpuSnap{time.Now(), map[int32]float64{}}
	elapsed := now.at.Sub(prev.at).Seconds()

	type pinfo struct {
		p    *process.Process
		ppid int32
		exe  string
	}
	info := map[int32]pinfo{}
	for _, p := range procs {
		exe, _ := p.Exe()
		if exe == "" {
			exe, _ = p.Name()
		}
		ppid, _ := p.Ppid()
		info[p.Pid] = pinfo{p, ppid, exe}
	}
	root := func(pid int32) int32 {
		for {
			i := info[pid]
			parent, ok := info[i.ppid]
			if !ok || i.ppid == pid || parent.exe != i.exe {
				return pid
			}
			pid = i.ppid
		}
	}

	groups := map[string]*appRow{}
	for _, p := range procs {
		var busy float64
		if ct, err := p.Times(); err == nil {
			now.times[p.Pid] = ct.User + ct.System
			if before, ok := prev.times[p.Pid]; ok && elapsed > 0 {
				busy = (now.times[p.Pid] - before) / elapsed * 100
			}
		}
		m := memOf(p)
		if m == 0 { // kernel threads
			continue
		}
		var a appRow
		var key string
		if c, ok := dk.containerOf(p.Pid); ok {
			a = appRow{Name: "docker:" + c.name, Folder: c.folder, Docker: true}
			key = a.Name
		} else {
			r := info[root(p.Pid)].p
			key = fmt.Sprint(r.Pid)
			if groups[key] == nil { // only look up name and folder once per app
				a = appRow{Name: procName(r), Folder: procFolder(r)}
			}
		}
		g := groups[key]
		if g == nil {
			g = &a
			groups[key] = g
		}
		g.Mem += m
		g.CPU += busy
		g.Procs++
	}

	apps := make([]appRow, 0, len(groups))
	for _, g := range groups {
		apps = append(apps, *g)
	}
	return apps, now
}

func sortApps(apps []appRow, byCPU bool) {
	sort.Slice(apps, func(i, j int) bool {
		if byCPU && apps[i].CPU != apps[j].CPU {
			return apps[i].CPU > apps[j].CPU
		}
		return apps[i].Mem > apps[j].Mem
	})
}

// memOf uses PSS on Linux, which splits shared memory fairly between
// processes; summing RSS over-counts multi-process apps like browsers.
// PSS needs sudo for other users' processes; RSS is the fallback everywhere.
func memOf(p *process.Process) uint64 {
	if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", p.Pid)); err == nil {
		if pss := parsePss(b); pss > 0 {
			return pss
		}
	}
	if m, err := p.MemoryInfo(); err == nil {
		return m.RSS
	}
	return 0
}

func parsePss(b []byte) uint64 {
	for _, line := range bytes.Split(b, []byte("\n")) {
		if f := bytes.Fields(line); len(f) >= 2 && string(f[0]) == "Pss:" {
			kb, _ := strconv.ParseUint(string(f[1]), 10, 64)
			return kb * 1024
		}
	}
	return 0
}

func human(b uint64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.0f MB", float64(b)/(1<<20))
	}
	return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
}
