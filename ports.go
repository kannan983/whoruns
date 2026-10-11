package main

import (
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	psnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

type portRow struct {
	Port    uint32
	Proto   string
	Addr    string
	PID     int32
	Name    string
	User    string
	Folder  string // absolute; shortened with ~ only when shown
	Command string
	Docker  bool
	ips     []string
}

func listPorts(filter uint32) ([]portRow, error) {
	conns, err := psnet.Connections("inet")
	if err != nil {
		return nil, err
	}
	// one socket per (proto, port, pid); IPv4 and IPv6 binds are merged
	socks := map[string]*portRow{}
	for _, c := range conns {
		var proto string
		switch {
		case c.Type == syscall.SOCK_STREAM && c.Status == "LISTEN":
			proto = "tcp"
		case c.Type == syscall.SOCK_DGRAM && c.Raddr.Port == 0:
			proto = "udp"
		default:
			continue
		}
		if c.Laddr.Port == 0 || (filter != 0 && c.Laddr.Port != filter) {
			continue
		}
		key := fmt.Sprint(proto, c.Laddr.Port, c.Pid)
		if socks[key] == nil {
			socks[key] = &portRow{Port: c.Laddr.Port, Proto: proto, PID: c.Pid}
		}
		socks[key].ips = append(socks[key].ips, c.Laddr.IP)
	}

	dk := loadDocker()
	// docker runs one docker-proxy per IP family, so merge again once the
	// owner is known: same port + same app + same folder is one row
	merged := map[string]*portRow{}
	for _, r := range socks {
		fillProc(r, dk)
		if c, ok := dk.ports[fmt.Sprint(r.Port, "/", r.Proto)]; ok {
			r.Name, r.Folder, r.Command, r.Docker = "docker:"+c.name, c.folder, c.image, true
		}
		key := fmt.Sprint(r.Proto, r.Port, r.Name, r.Folder)
		if m := merged[key]; m != nil {
			m.ips = append(m.ips, r.ips...)
			continue
		}
		merged[key] = r
	}

	rows := make([]portRow, 0, len(merged))
	for _, r := range merged {
		r.Addr = addrLabel(r.ips)
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Port != rows[j].Port {
			return rows[i].Port < rows[j].Port
		}
		return rows[i].Proto < rows[j].Proto
	})
	return rows, nil
}

// fillProc looks up the owning process. Each field is best effort: a process
// owned by another user (without sudo) only yields what the OS exposes.
func fillProc(r *portRow, dk dockerInfo) {
	if r.PID == 0 {
		return
	}
	p, err := process.NewProcess(r.PID)
	if err != nil {
		return
	}
	r.User, _ = p.Username()
	if c, ok := dk.containerOf(r.PID); ok { // host-network container: its cwd is inside the container
		r.Name, r.Folder, r.Command, r.Docker = "docker:"+c.name, c.folder, c.image, true
		return
	}
	r.Name = procName(p)
	r.Folder = procFolder(p)
	r.Command, _ = p.Cmdline()
}

// procName is the program name from the command line: the OS process name
// can be a thread name (node shows "MainThread"). Chromium-based apps rewrite
// argv into "/opt/app/app --type=utility ...", so only the first word counts.
func procName(p *process.Process) string {
	name, _ := p.Name()
	if args, err := p.CmdlineSlice(); err == nil && len(args) > 0 && args[0] != "" {
		name = args[0]
	}
	if f := strings.Fields(name); len(f) > 0 {
		name = filepath.Base(f[0])
	}
	return strings.TrimSuffix(name, ".exe")
}

func procFolder(p *process.Process) string {
	if cwd, err := p.Cwd(); err == nil && cwd != "" {
		return cwd
	}
	if exe, err := p.Exe(); err == nil && exe != "" { // Windows has no cwd lookup
		return filepath.Dir(exe)
	}
	return ""
}

func addrLabel(ips []string) string {
	all, local := false, true
	seen := map[string]bool{}
	var uniq []string
	for _, s := range ips {
		ip := net.ParseIP(s)
		if ip == nil {
			continue
		}
		all = all || ip.IsUnspecified()
		local = local && ip.IsLoopback()
		if !seen[s] {
			seen[s] = true
			uniq = append(uniq, s)
		}
	}
	switch {
	case all:
		return "all"
	case local && len(uniq) > 0:
		return "local"
	}
	return strings.Join(uniq, ",")
}
