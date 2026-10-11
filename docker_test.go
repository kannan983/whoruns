package main

import "testing"

func TestParseDockerPS(t *testing.T) {
	out := "abc123def456\tweb\t0.0.0.0:3000->3000/tcp, [::]:3000->3000/tcp, 8920/tcp\t/home/k/app\tnode:22\n" +
		"0123456789ab\tdns\t127.0.0.1:5300-5301->53-54/udp\t\tcoredns\n" +
		"ffffffffffff\tidle\t\t\tbusybox\n"
	d := parseDockerPS(out)
	if c := d.ports["3000/tcp"]; c.name != "web" || c.folder != "/home/k/app" || c.image != "node:22" {
		t.Errorf("3000/tcp = %+v", c)
	}
	if d.ports["5300/udp"].name != "dns" || d.ports["5301/udp"].name != "dns" {
		t.Errorf("range not expanded: %+v", d.ports)
	}
	if _, ok := d.ports["8920/tcp"]; ok {
		t.Error("unpublished port should be ignored")
	}
	if len(d.ports) != 3 || d.ids["ffffffffffff"].name != "idle" {
		t.Errorf("got %+v", d)
	}
}

func TestAddrLabel(t *testing.T) {
	for _, tc := range []struct {
		ips  []string
		want string
	}{
		{[]string{"127.0.0.1", "::1"}, "local"},
		{[]string{"0.0.0.0", "::"}, "all"},
		{[]string{"100.1.2.3", "100.1.2.3"}, "100.1.2.3"},
	} {
		if got := addrLabel(tc.ips); got != tc.want {
			t.Errorf("addrLabel(%v) = %q, want %q", tc.ips, got, tc.want)
		}
	}
}

func TestPss(t *testing.T) {
	b := []byte("55d0-7ffc ---p 00000000 00:00 0 [rollup]\nRss:  2048 kB\nPss:  1024 kB\nPss_Anon: 10 kB\n")
	if got := parsePss(b); got != 1024*1024 {
		t.Errorf("parsePss = %d", got)
	}
}

func TestParseDRM(t *testing.T) {
	i915 := "drm-driver:\ti915\ndrm-client-id:\t7\ndrm-pdev:\t0000:00:02.0\n" +
		"drm-resident-system0:\t2048 KiB\ndrm-engine-render:\t5000 ns\ndrm-engine-video:\t10 ns\n" +
		"drm-engine-capacity-video:\t2\n"
	key, c := parseDRM([]byte(i915))
	if key != "0000:00:02.0/7" || c.mem != 2<<20 || c.engines["render"] != 5000 || c.capacity["video"] != 2 {
		t.Fatalf("i915: key=%q client=%+v", key, c)
	}
	if _, ok := c.engines["capacity-video"]; ok {
		t.Error("capacity parsed as an engine")
	}
	// older amdgpu: memory as drm-memory-*, no resident keys
	_, c = parseDRM([]byte("drm-client-id: 3\ndrm-pdev: 0000:03:00.0\ndrm-memory-vram: 1024 KiB\ndrm-engine-gfx: 1 ns\n"))
	if c.mem != 1<<20 {
		t.Errorf("amdgpu mem = %d", c.mem)
	}
	if key, _ := parseDRM([]byte("pos: 0\nflags: 02\n")); key != "" {
		t.Error("non-DRM fdinfo should be ignored")
	}

	prev := &drmClient{engines: map[string]uint64{"render": 0, "video": 0}, capacity: map[string]float64{}}
	now := &drmClient{engines: map[string]uint64{"render": 500e6, "video": 1e9}, capacity: map[string]float64{"video": 2}}
	busy := gpuBusy(now, prev, 1e9)
	if busy["render"] != 50 || busy["video"] != 50 {
		t.Errorf("gpuBusy = %v", busy)
	}
}

func TestParsePmon(t *testing.T) {
	out := "# gpu         pid   type     sm    mem    enc    dec    jpg    ofa     fb   command\n" +
		"# Idx           #    C/G      %      %      %      %      %      %     MB   name\n" +
		"    0       1234     G     12      2      -      -      -      -    120   Xorg\n" +
		"    0       5678     C      -      -      -      -      -      -     64   python\n"
	p := parsePmon(out)
	if p[1234].sm != 12 || p[1234].mem != 120<<20 || p[5678].sm != 0 || p[5678].mem != 64<<20 {
		t.Errorf("parsePmon = %+v", p)
	}
}
