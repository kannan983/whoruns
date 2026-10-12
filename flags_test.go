package main

import (
	"slices"
	"testing"
)

func TestSuspicious(t *testing.T) {
	cases := []struct {
		exe  string
		args []string
		want []string
	}{
		{"/usr/bin/sshd", []string{"/usr/bin/sshd", "-D"}, nil},
		{"/tmp/.x/payload", []string{"./payload"}, []string{"tmp-exe"}},
		{"/dev/shm/a (deleted)", []string{"a"}, []string{"deleted", "tmp-exe"}},
		{"/home/u/miner (deleted)", []string{"[kworker/0:1]"}, []string{"deleted", "fake-kthread"}},
		{"/opt/tmp/app", []string{"app"}, nil}, // only the temp dirs themselves count
		{"", nil, nil},                         // kernel thread or no permission
	}
	for _, c := range cases {
		if got := suspicious(c.exe, c.args); !slices.Equal(got, c.want) {
			t.Errorf("suspicious(%q, %q) = %q, want %q", c.exe, c.args, got, c.want)
		}
	}
}
