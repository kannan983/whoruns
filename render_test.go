package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestTableFitsTerminal(t *testing.T) {
	plain := func(int, int) lipgloss.Style { return lipgloss.NewStyle() }
	for _, w := range []int{40, 52, 60, 80, 100, 120, 160} {
		for _, window := range []*[2]int{nil, {0, 1}} {
			cells := [][]string{{"1883", "tcp", "100.119.87.58,fd7a:115c", "-", "docker:some-long-container-name",
				"kannan", "deleted", "/home/kannan/Projects/x/mosquitto", "/usr/bin/eclipse-mosquitto -c /etc/x.conf"}}
			out := renderTable(portHeaders, cells, 7, 8, w, window, plain)
			for _, line := range strings.Split(out, "\n") {
				if got := lipgloss.Width(line); got > w {
					t.Fatalf("width %d (window %v): line is %d wide: %q", w, window, got, line)
				}
			}
		}
	}
}
