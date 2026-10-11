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
