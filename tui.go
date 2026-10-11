package main

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const refresh = 2 * time.Second

type dataMsg struct {
	ports []portRow
	apps  []appRow
	snap  cpuSnap
	sys   sysInfo
}

type tickMsg struct{}

type model struct {
	tab    int // 0 ports, 1 apps
	byCPU  bool
	ports  []portRow
	apps   []appRow
	snap   cpuSnap
	sys    sysInfo
	shown  []int // indexes into ports/apps for the rows that match the search
	cursor int   // position in shown
	offset int   // first visible position in shown
	search textinput.Model
	status string
	w, h   int
}

func runTUI() error {
	s := textinput.New()
	s.Prompt = "/ "
	s.Placeholder = "search app, port, folder…"
	_, err := tea.NewProgram(model{search: s, snap: snapCPU()}).Run()
	return err
}

func fetch(prev cpuSnap) tea.Cmd {
	return func() tea.Msg {
		ports, _ := listPorts(0)
		apps, snap := listApps(prev)
		return dataMsg{ports, apps, snap, sysStats(false)}
	}
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

// the first CPU sample needs a gap after the snapshot taken in runTUI
func (m model) Init() tea.Cmd { return tick(500 * time.Millisecond) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		return m, fetch(m.snap)
	case dataMsg:
		m.ports, m.apps, m.snap, m.sys = msg.ports, msg.apps, msg.snap, msg.sys
		m.filter()
		return m, tick(refresh)
	case tea.KeyPressMsg:
		if m.search.Focused() {
			switch msg.String() {
			case "esc":
				m.search.SetValue("")
				m.search.Blur()
			case "enter":
				m.search.Blur()
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.cursor = 0
				m.filter()
				return m, cmd
			}
			m.filter()
			return m, nil
		}
		m.status = ""
		page := max(m.rowsVisible()-1, 1)
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab", "1", "2":
			if msg.String() == "tab" {
				m.tab = 1 - m.tab
			} else {
				m.tab = int(msg.String()[0] - '1')
			}
			m.cursor = 0
			m.filter()
		case "/":
			return m, m.search.Focus()
		case "esc":
			m.search.SetValue("")
			m.filter()
		case "s":
			m.byCPU = !m.byCPU
			m.filter()
		case "c", "y":
			return m.copy(msg.String() == "y")
		case "up", "k":
			m.cursor--
		case "down", "j":
			m.cursor++
		case "pgup", "ctrl+u":
			m.cursor -= page
		case "pgdown", "ctrl+d", "space":
			m.cursor += page
		case "home", "g":
			m.cursor = 0
		case "end", "G":
			m.cursor = len(m.shown) - 1
		}
	}
	m.scroll()
	return m, nil
}

func (m model) copy(command bool) (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.shown) {
		return m, nil
	}
	i := m.shown[m.cursor]
	var text, what string
	switch {
	case m.tab == 0 && command:
		text, what = m.ports[i].Command, "command"
	case m.tab == 0:
		text, what = m.ports[i].Folder, "folder"
	case !command:
		text, what = m.apps[i].Folder, "folder"
	}
	if text == "" {
		m.status = yellow.Render("nothing to copy")
		return m, nil
	}
	m.status = green.Render("✓ copied "+what) + dim.Render("  "+clip(text, 60))
	return m, tea.SetClipboard(text)
}

// filter recomputes which rows match the search.
func (m *model) filter() {
	var cells [][]string
	if m.tab == 0 {
		cells = portCells(m.ports)
	} else {
		sortApps(m.apps, m.byCPU)
		cells = appCells(m.apps)
	}
	q := strings.ToLower(m.search.Value())
	m.shown = m.shown[:0]
	for i, row := range cells {
		if q == "" || strings.Contains(strings.ToLower(strings.Join(row, " ")), q) {
			m.shown = append(m.shown, i)
		}
	}
	m.scroll()
}

// rows that fit: the screen minus title, gap, footer and 4 table border lines
func (m model) rowsVisible() int { return max(m.h-7, 1) }

func (m *model) scroll() {
	m.cursor = min(max(m.cursor, 0), max(len(m.shown)-1, 0))
	n := m.rowsVisible()
	m.offset = min(max(m.offset, m.cursor-n+1), m.cursor)
	m.offset = max(min(m.offset, len(m.shown)-n), 0)
}

func (m model) View() tea.View {
	pad := lipgloss.NewStyle().Padding(0, 1)
	v := tea.NewView(pad.Render(m.titleBar() + "\n\n" + m.tableView() + "\n" + m.footer()))
	v.AltScreen = true
	return v
}

func (m model) titleBar() string {
	pill := lipgloss.NewStyle().Padding(0, 1)
	left := pill.Bold(true).Foreground(accent).Render("◉ whoruns")
	for i, t := range []string{"Ports", "Apps"} {
		label := fmt.Sprintf("%d %s", i+1, t)
		if i == m.tab {
			left += " " + pill.Inherit(selected).Render(label)
		} else {
			left += " " + pill.Foreground(lipgloss.Color("8")).Render(label)
		}
	}
	if m.sys.total == 0 {
		return left
	}
	right := sysLine(m.sys)
	gap := max(m.w-2-lipgloss.Width(left)-lipgloss.Width(right), 2)
	return left + strings.Repeat(" ", gap) + right
}

func (m model) tableView() string {
	if m.ports == nil && m.apps == nil {
		return dim.Render("  loading…")
	}
	var headers []string
	var all [][]string
	var style func(row, col int) lipgloss.Style
	folderCol, cmdCol := 6, 7
	if m.tab == 0 {
		headers, all, style = portHeaders, portCells(m.ports), portStyle(m.ports)
	} else {
		headers, all, style = appHeaders, appCells(m.apps), appStyle(m.apps)
		folderCol, cmdCol = 4, -1
	}
	if len(m.shown) == 0 {
		cells := [][]string{make([]string, len(headers))}
		cells[0][0] = "no matches"
		return renderTable(headers, cells, folderCol, cmdCol, m.w-2, &[2]int{0, 1},
			func(int, int) lipgloss.Style { return dim })
	}
	cells := make([][]string, len(m.shown))
	for i, idx := range m.shown {
		cells[i] = all[idx]
	}
	window := [2]int{m.offset, min(m.offset+m.rowsVisible(), len(m.shown))}
	return renderTable(headers, cells, folderCol, cmdCol, m.w-2, &window, func(row, col int) lipgloss.Style {
		if m.offset+row == m.cursor {
			return selected
		}
		return style(m.shown[m.offset+row], col)
	})
}

func (m model) footer() string {
	count := dim.Render(fmt.Sprintf("%d/%d ", min(m.cursor+1, len(m.shown)), len(m.shown)))
	switch {
	case m.search.Focused() || m.search.Value() != "":
		return count + m.search.View()
	case m.status != "":
		return count + m.status
	}
	keys := [][2]string{{"tab", "switch"}, {"/", "search"}, {"c", "copy folder"}}
	if m.tab == 0 {
		keys = append(keys, [2]string{"y", "copy command"})
	} else if m.byCPU {
		keys = append(keys, [2]string{"s", "sort by memory"})
	} else {
		keys = append(keys, [2]string{"s", "sort by cpu"})
	}
	keys = append(keys, [2]string{"q", "quit"})
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = header.Render(k[0]) + " " + dim.Render(k[1])
	}
	return count + strings.Join(parts, dim.Render("  ·  "))
}
