package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type container struct {
	name   string
	folder string
	image  string
}

type dockerInfo struct {
	ports map[string]container // "PORT/proto" -> container publishing it
	ids   map[string]container // short (12 char) container ID -> container
}

// "0.0.0.0:8000-8001->8000-8001/tcp" -> host ports 8000..8001, tcp
var publishedRe = regexp.MustCompile(`:(\d+)(?:-(\d+))?->\d+(?:-\d+)?/(tcp|udp)`)

// cgroup v1 "/docker/<id>", v2 "docker-<id>.scope" (rootless too)
var cgroupRe = regexp.MustCompile(`docker[-/]([0-9a-f]{64})`)

// loadDocker asks docker which containers run where. A published port is held
// by docker-proxy (or the Docker Desktop VM), whose own folder is useless, so
// the compose working_dir label is where the project lives. Empty if docker
// is missing or not running.
func loadDocker() dockerInfo {
	out, err := exec.Command("docker", "ps", "--format",
		`{{.ID}}	{{.Names}}	{{.Ports}}	{{.Label "com.docker.compose.project.working_dir"}}	{{.Image}}`).Output()
	if err != nil {
		return dockerInfo{}
	}
	return parseDockerPS(string(out))
}

func parseDockerPS(out string) dockerInfo {
	d := dockerInfo{ports: map[string]container{}, ids: map[string]container{}}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 5 {
			continue
		}
		c := container{name: f[1], folder: f[3], image: f[4]}
		d.ids[f[0]] = c
		for _, p := range publishedRe.FindAllStringSubmatch(f[2], -1) {
			lo, _ := strconv.Atoi(p[1])
			hi := lo
			if p[2] != "" {
				hi, _ = strconv.Atoi(p[2])
			}
			for port := lo; port <= hi; port++ {
				d.ports[fmt.Sprint(port, "/", p[3])] = c
			}
		}
	}
	return d
}

// containerOf reports which container a process runs in (Linux only; the
// cgroup file does not exist elsewhere).
func (d dockerInfo) containerOf(pid int32) (container, bool) {
	if len(d.ids) == 0 {
		return container{}, false
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return container{}, false
	}
	m := cgroupRe.FindSubmatch(b)
	if m == nil {
		return container{}, false
	}
	c, ok := d.ids[string(m[1][:12])]
	return c, ok
}
