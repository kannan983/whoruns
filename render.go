package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"golang.org/x/term"
)

// ANSI palette indexes, so colours follow the terminal's own theme
var (
	dim    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	bold   = lipgloss.NewStyle().Bold(true)
	cyan   = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	blue   = lipgloss.NewStyle().Foreground(lipgloss.Color("4"))
	yellow = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	red    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	green  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))

	accent   = lipgloss.Color("#8B7CF6")
	header   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	selected = lipgloss.NewStyle().Bold(true).Background(accent).Foreground(lipgloss.Color("#FFFFFF"))
)

var (
	portHeaders = []string{"PORT", "PROTO", "ADDR", "PID", "APP", "USER", "FOLDER", "COMMAND"}
	appHeaders  = []string{"APP", "MEMORY", "CPU", "GPU", "GPU MEM", "PROCS", "FOLDER"}
)

func portCells(rows []portRow) [][]string {
	home, _ := os.UserHomeDir()
	out := make([][]string, len(rows))
	for i, r := range rows {
		pid := ""
		if r.PID != 0 {
			pid = strconv.Itoa(int(r.PID))
		}
		name := dash(r.Name)
		if r.Port == 1337 {
			name += " ⚡elite"
		}
		out[i] = []string{strconv.Itoa(int(r.Port)), r.Proto, clip(r.Addr, 16), dash(pid),
			name, dash(r.User), dash(tilde(r.Folder, home)), dash(r.Command)}
	}
	return out
}

func appCells(apps []appRow) [][]string {
	home, _ := os.UserHomeDir()
	out := make([][]string, len(apps))
	for i, a := range apps {
		gpu, gpuMem := "-", "-"
		if a.HasGPU {
			gpu = fmt.Sprintf("%.0f%%", a.GPU)
			if a.GPUMem > 0 {
				gpuMem = human(a.GPUMem)
			}
		}
		out[i] = []string{a.Name, human(a.Mem), fmt.Sprintf("%.0f%%", a.CPU), gpu, gpuMem,
			strconv.Itoa(a.Procs), dash(tilde(a.Folder, home))}
	}
	return out
}

func printPorts(rows []portRow) {
	hidden := 0
	for _, r := range rows {
		if r.Name == "" {
			hidden++
		}
	}
	lipgloss.Println(dim.Render(fmt.Sprintf(" %d listening ports", len(rows))))
	lipgloss.Println(renderTable(portHeaders, portCells(rows), 6, 7, cliWidth(), nil, portStyle(rows)))
	if hidden > 0 {
		lipgloss.Println(dim.Render(fmt.Sprintf(" %d owned by other users: run with sudo to see them", hidden)))
	}
}

func printApps(apps []appRow, sys sysInfo, by string) {
	if len(apps) > 15 {
		apps = apps[:15]
	}
	lipgloss.Println(" " + sysLine(sys) + dim.Render("  ·  top apps by "+sortLabel[by]))
	lipgloss.Println(renderTable(appHeaders, appCells(apps), 6, -1, cliWidth(), nil, appStyle(apps)))
}

func portStyle(rows []portRow) func(row, col int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		r := rows[row]
		switch {
		case r.Port == 1337 && (col == 0 || col == 4):
			return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#39FF14"))
		case r.Name == "":
			return dim
		case col == 0:
			return cyan.Bold(true)
		case r.Docker && col == 4:
			return blue
		}
		return lipgloss.NewStyle()
	}
}

func appStyle(apps []appRow) func(row, col int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		a := apps[row]
		switch col {
		case 0:
			if a.Docker {
				return blue.Bold(true)
			}
			return bold
		case 1:
			return heat(float64(a.Mem), 500<<20, 2<<30)
		case 2:
			return heat(a.CPU, 10, 50)
		case 3:
			return heat(a.GPU, 10, 50)
		}
		return lipgloss.NewStyle()
	}
}

func sysLine(s sysInfo) string {
	pct := 0.0
	if s.total > 0 {
		pct = float64(s.used) / float64(s.total) * 100
	}
	gpuPart := ""
	if s.hasGPU {
		gpuPart = dim.Render("  ·  ") + bold.Render("GPU ") + heat(s.gpu, 50, 85).Render(fmt.Sprintf("%.0f%%", s.gpu))
	}
	return bold.Render("Memory ") + heat(pct, 60, 85).Render(fmt.Sprintf("%s / %s", human(s.used), human(s.total))) +
		dim.Render("  ·  ") + bold.Render("CPU ") + heat(s.cpu, 50, 85).Render(fmt.Sprintf("%.0f%%", s.cpu)) +
		gpuPart
}

var sortLabel = map[string]string{"mem": "memory", "cpu": "CPU", "gpu": "GPU"}

func heat(v, warn, hot float64) lipgloss.Style {
	switch {
	case v >= hot:
		return red.Bold(true)
	case v >= warn:
		return yellow
	}
	return lipgloss.NewStyle()
}

// renderTable fits the cells into width and draws them. The CLI draws every
// row with column lines; the TUI (window) draws rows[from:to] without them, so
// the selected row reads as one solid bar, and pins each column to the width
// of all rows so nothing jumps while scrolling. Style rows are window-relative.
func renderTable(headers []string, cells [][]string, folderCol, cmdCol, width int, window *[2]int, style func(row, col int) lipgloss.Style) string {
	overhead := 2*len(headers) + 2 // cell padding + outer border
	if window == nil {
		overhead += len(headers) - 1
	}
	widths := fitCells(headers, cells, folderCol, cmdCol, width-overhead)
	if window != nil {
		used := 0
		for _, w := range widths {
			used += w
		}
		widths[folderCol] += max(width-overhead-used, 0) // spare room goes to the folder
		headers = append([]string(nil), headers...)
		for i := range headers {
			headers[i] += strings.Repeat(" ", widths[i]-lipgloss.Width(headers[i]))
		}
		cells = cells[window[0]:window[1]]
	}
	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(dim).
		BorderColumn(window == nil).
		Headers(headers...).
		Rows(cells...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return header.Padding(0, 1)
			}
			return style(row, col).Padding(0, 1)
		}).
		String()
}

func cliWidth() int {
	if w := termWidth(); w > 0 {
		return w
	}
	return 160 // piped: still keep lines readable
}

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}

// fitCells shrinks the command, then the folder column, until the
// table fits avail columns, and returns the final widths. Folders lose their
// start ("…/Active/webapp"), commands lose their end.
func fitCells(headers []string, cells [][]string, folderCol, cmdCol, avail int) []int {
	w := make([]int, len(headers))
	for i, h := range headers {
		w[i] = lipgloss.Width(h)
	}
	for _, row := range cells {
		for i, c := range row {
			w[i] = max(w[i], lipgloss.Width(c))
		}
	}
	total := 0
	for _, x := range w {
		total += x
	}
	for total > avail {
		col := cmdCol // the command gives way first, the folder is the point
		if col < 0 || w[col] <= 12 {
			col = folderCol
		}
		if w[col] <= 12 {
			break
		}
		w[col]--
		total--
	}
	for _, row := range cells {
		row[folderCol] = clipLeft(row[folderCol], w[folderCol])
		if cmdCol >= 0 {
			row[cmdCol] = clip(row[cmdCol], w[cmdCol])
		}
	}
	return w
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func clipLeft(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return "…" + string(r[len(r)-n+1:])
	}
	return s
}

func tilde(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+string(filepath.Separator))) {
		return "~" + path[len(home):]
	}
	return path
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// theWorld answers `whoruns the world`.
func theWorld() {
	gold := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
	lipgloss.Println(gold.Render(`
     .     *     .
    /\   /\   /\
   /  \_/  \_/  \
   \____________/`))
	lipgloss.Println("   Who runs the world? " + header.Render("Girls."))
	lipgloss.Println(dim.Render("   (for ports, try plain `whoruns`)"))
}
