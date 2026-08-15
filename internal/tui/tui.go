package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/easysftp/uplift/internal/config"
	"github.com/easysftp/uplift/internal/output"
	"github.com/easysftp/uplift/internal/uploader"
)

type screen int

const (
	screenProfiles screen = iota
	screenFiles
	screenUploading
	screenSummary
	screenAddProfile
	screenConfirmDelete
	screenEditProfile
	screenConfirmUpload
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).PaddingBottom(1)
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	selectStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	checkStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	crossStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)
	purpleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("135"))
	localInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	remoteInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
)

type Model struct {
	cfg          *config.Config
	configPath   string
	current      screen
	profiles     []string
	cursor       int
	selectedProf int
	files        []string
	selected     map[int]bool
	results      []uploader.Result
	uploaded     int
	skipped      int
	failed       int
	up           *uploader.Uploader
	writer       *output.Writer
	concurrency  int
	inputs       []textinput.Model
	inputIdx     int
	formLabels   []string
	errMsg       string
	confirmMsg   string
	confirmAction string
	editName     string
	scrollOffset int
	pendingJobs  []uploader.Job
	remoteStats  map[string]remoteStatInfo
	batchOffset  int
	allResults   []uploader.Result
	allUploaded  int
	allSkipped   int
	allFailed    int
	sessionLog   *uploader.SessionLog
	batchEnabled bool
}

func New(cfg *config.Config, configPath string) *Model {
	m := &Model{
		cfg:          cfg,
		configPath:   configPath,
		current:      screenProfiles,
		selected:     make(map[int]bool),
		concurrency:  1,
		writer:       output.New(os.Stdout),
		batchEnabled: true,
	}
	m.profiles = cfg.Names()
	sort.Strings(m.profiles)
	return m
}

func (m *Model) Init() tea.Cmd {
	return nil
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(uploadResultMsg); ok {
		m.allResults = append(m.allResults, result.results...)
		m.allUploaded += result.uploaded
		m.allSkipped += result.skipped
		m.allFailed += result.failed

		m.batchOffset += batchSize
		if !m.batchEnabled || m.batchOffset >= len(m.pendingJobs) {
			if m.sessionLog != nil {
				m.sessionLog.Close()
			}
			m.results = m.allResults
			m.uploaded = m.allUploaded
			m.skipped = m.allSkipped
			m.failed = m.allFailed
			m.current = screenSummary
			return m, nil
		}

		m.remoteStats = nil
		m.current = screenConfirmUpload
		nextBatch := m.currentBatch()
		return m, m.fetchRemoteStats(nextBatch, m.batchOffset)
	}

	if stats, ok := msg.(remoteStatsMsg); ok {
		m.remoteStats = stats.stats
		return m, nil
	}

	switch m.current {
	case screenProfiles:
		return m.updateProfiles(msg)
	case screenFiles:
		return m.updateFiles(msg)
	case screenUploading:
		return m.updateUploading(msg)
	case screenSummary:
		return m.updateSummary(msg)
	case screenAddProfile:
		return m.updateAddProfile(msg)
	case screenConfirmDelete:
		return m.updateConfirmDelete(msg)
	case screenEditProfile:
		return m.updateEditProfile(msg)
	case screenConfirmUpload:
		return m.updateConfirmUpload(msg)
	}
	return m, nil
}

func (m *Model) updateProfiles(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.profiles)-1 {
				m.cursor++
			}
		case "enter", " ":
			if len(m.profiles) > 0 {
				m.selectedProf = m.cursor
				m.current = screenFiles
				m.loadFiles()
			}
		case "a":
			m.current = screenAddProfile
			m.formLabels = []string{"Profile name", "Host", "Port", "Username", "Private key", "Local base path", "Remote base path"}
			m.inputs = make([]textinput.Model, len(m.formLabels))
			for i := range m.inputs {
				ti := textinput.New()
				ti.Placeholder = m.formLabels[i]
				if i == 2 {
					ti.SetValue("22")
				}
				m.inputs[i] = ti
			}
			m.inputIdx = 0
			m.inputs[0].Focus()
		case "d":
			if len(m.profiles) > 0 {
				m.confirmMsg = fmt.Sprintf("Delete profile %q?", m.profiles[m.cursor])
				m.confirmAction = "delete"
				m.current = screenConfirmDelete
			}
		case "e":
			if len(m.profiles) > 0 {
				m.editName = m.profiles[m.cursor]
				m.startEditProfile(m.editName)
			}
		}
	}
	return m, nil
}

func (m *Model) updateFiles(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.current = screenProfiles
			m.scrollOffset = 0
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				if m.cursor < m.scrollOffset {
					m.scrollOffset = m.cursor
				}
			}
		case "down", "j":
			if m.cursor < len(m.files)-1 {
				m.cursor++
				visibleEnd := m.scrollOffset + 15
				if m.cursor >= visibleEnd {
					m.scrollOffset = m.cursor - 14
				}
			}
		case " ":
			if _, ok := m.selected[m.cursor]; ok {
				delete(m.selected, m.cursor)
			} else {
				m.selected[m.cursor] = true
			}
		case "b":
			m.batchEnabled = !m.batchEnabled
		case "enter":
			if len(m.selected) > 0 {
				return m, m.prepareUpload()
			}
		}
	}
	return m, nil
}

func (m *Model) updateUploading(msg tea.Msg) (tea.Model, tea.Cmd) {
	if len(m.results) == 0 {
		m.current = screenSummary
	}
	return m, nil
}

func (m *Model) updateAddProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.current = screenProfiles
			return m, nil
		case "up", "shift+tab":
			m.inputs[m.inputIdx].Blur()
			if m.inputIdx > 0 {
				m.inputIdx--
			}
			m.inputs[m.inputIdx].Focus()
			return m, nil
		case "down", "tab", "enter":
			m.inputs[m.inputIdx].Blur()
			if m.inputIdx < len(m.inputs)-1 {
				m.inputIdx++
				m.inputs[m.inputIdx].Focus()
			} else {
				m.saveProfile()
			}
			return m, nil
		}
	}

	m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
	return m, cmd
}

func (m *Model) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "n", "N":
			m.current = screenProfiles
		case "y", "Y", "enter":
			m.deleteSelectedProfile()
		}
	}
	return m, nil
}

func (m *Model) startEditProfile(name string) {
	p, ok := m.cfg.Get(name)
	if !ok {
		return
	}

	m.formLabels = []string{"Profile name", "Host", "Port", "Username", "Private key", "Local base path", "Remote base path"}
	m.inputs = make([]textinput.Model, len(m.formLabels))
	values := []string{
		name,
		p.Host,
		fmt.Sprintf("%d", p.Port),
		p.Username,
		p.KeyFile,
		p.LocalBase,
		p.RemoteBase,
	}
	for i := range m.inputs {
		ti := textinput.New()
		ti.Placeholder = m.formLabels[i]
		ti.SetValue(values[i])
		m.inputs[i] = ti
	}
	m.inputIdx = 0
	m.inputs[0].Focus()
	m.current = screenEditProfile
}

func (m *Model) updateEditProfile(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.current = screenProfiles
			return m, nil
		case "up", "shift+tab":
			m.inputs[m.inputIdx].Blur()
			if m.inputIdx > 0 {
				m.inputIdx--
			}
			m.inputs[m.inputIdx].Focus()
			return m, nil
		case "down", "tab", "enter":
			m.inputs[m.inputIdx].Blur()
			if m.inputIdx < len(m.inputs)-1 {
				m.inputIdx++
				m.inputs[m.inputIdx].Focus()
			} else {
				m.saveEditedProfile()
			}
			return m, nil
		}
	}

	m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
	return m, cmd
}

func (m *Model) saveEditedProfile() {
	newName := strings.TrimSpace(m.inputs[0].Value())
	host := strings.TrimSpace(m.inputs[1].Value())
	port := 22
	fmt.Sscanf(strings.TrimSpace(m.inputs[2].Value()), "%d", &port)
	username := strings.TrimSpace(m.inputs[3].Value())
	keyFile := strings.TrimSpace(m.inputs[4].Value())
	localBase := strings.TrimSpace(m.inputs[5].Value())
	remoteBase := strings.TrimSpace(m.inputs[6].Value())

	profile := config.Profile{
		Host:       host,
		Port:       port,
		Username:   username,
		KeyFile:    keyFile,
		LocalBase:  localBase,
		RemoteBase: remoteBase,
	}
	profile.Normalize()

	if newName != m.editName {
		if err := m.cfg.Add(newName, profile); err != nil {
			m.errMsg = err.Error()
			return
		}
		if err := m.cfg.Remove(m.editName); err != nil {
			m.errMsg = err.Error()
			return
		}
	} else {
		if err := m.cfg.Update(m.editName, profile); err != nil {
			m.errMsg = err.Error()
			return
		}
	}

	if err := m.cfg.Save(m.configPath); err != nil {
		m.errMsg = err.Error()
		return
	}

	m.errMsg = ""
	m.profiles = m.cfg.Names()
	sort.Strings(m.profiles)
	m.current = screenProfiles
}

func (m *Model) deleteSelectedProfile() {
	if len(m.profiles) == 0 {
		m.current = screenProfiles
		return
	}
	name := m.profiles[m.cursor]
	if err := m.cfg.Remove(name); err != nil {
		m.errMsg = err.Error()
		m.current = screenProfiles
		return
	}
	if err := m.cfg.Save(m.configPath); err != nil {
		m.errMsg = err.Error()
		m.current = screenProfiles
		return
	}

	m.profiles = m.cfg.Names()
	sort.Strings(m.profiles)
	if m.cursor >= len(m.profiles) {
		m.cursor = len(m.profiles) - 1
	}
	m.current = screenProfiles
}

func (m *Model) updateSummary(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "enter":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *Model) saveProfile() {
	name := strings.TrimSpace(m.inputs[0].Value())
	host := strings.TrimSpace(m.inputs[1].Value())
	port := 22
	fmt.Sscanf(strings.TrimSpace(m.inputs[2].Value()), "%d", &port)
	username := strings.TrimSpace(m.inputs[3].Value())
	keyFile := strings.TrimSpace(m.inputs[4].Value())
	localBase := strings.TrimSpace(m.inputs[5].Value())
	remoteBase := strings.TrimSpace(m.inputs[6].Value())

	profile := config.Profile{
		Host:       host,
		Port:       port,
		Username:   username,
		KeyFile:    keyFile,
		LocalBase:  localBase,
		RemoteBase: remoteBase,
	}
	profile.Normalize()

	if err := m.cfg.Add(name, profile); err != nil {
		m.errMsg = err.Error()
		return
	}
	if err := m.cfg.Save(m.configPath); err != nil {
		m.errMsg = err.Error()
		return
	}

	m.errMsg = ""
	m.profiles = m.cfg.Names()
	sort.Strings(m.profiles)
	m.current = screenProfiles
}

func (m *Model) View() string {
	switch m.current {
	case screenProfiles:
		return m.viewProfiles()
	case screenFiles:
		return m.viewFiles()
	case screenUploading:
		return m.viewUploading()
	case screenSummary:
		return m.viewSummary()
	case screenAddProfile:
		return m.viewAddProfile()
	case screenConfirmDelete:
		return m.viewConfirmDelete()
	case screenEditProfile:
		return m.viewEditProfile()
	case screenConfirmUpload:
		return m.viewConfirmUpload()
	}
	return ""
}

func (m *Model) viewProfiles() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Uplift — choose a profile"))
	b.WriteString("\n")

	if len(m.profiles) == 0 {
		b.WriteString(dimStyle.Render("No profiles found. Press 'a' to add one."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("a add profile  q quit"))
		return b.String()
	}

	for i, name := range m.profiles {
		p := m.cfg.Profiles[name]
		cursor := "  "
		if i == m.cursor {
			cursor = selectStyle.Render("> ")
		}

		nameStr := name
		if i == m.cursor {
			nameStr = selectStyle.Render(name)
		}
		serverStr := dimStyle.Render(fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port))

		line := fmt.Sprintf("%s%s  %s", cursor, nameStr, serverStr)
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate  enter select  "))
	b.WriteString(checkStyle.Render("a add"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(localInfoStyle.Render("e edit"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(crossStyle.Render("d delete"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(purpleStyle.Render("q quit"))
	return b.String()
}

func (m *Model) viewAddProfile() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Uplift — add profile"))
	b.WriteString("\n")

	for i := range m.inputs {
		b.WriteString(m.formLabels[i])
		b.WriteString(": ")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
	}

	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(crossStyle.Render(m.errMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate  enter next  esc back  q quit"))
	return b.String()
}

func (m *Model) viewEditProfile() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Uplift — edit profile %q", m.editName)))
	b.WriteString("\n")

	for i := range m.inputs {
		b.WriteString(m.formLabels[i])
		b.WriteString(": ")
		b.WriteString(m.inputs[i].View())
		b.WriteString("\n")
	}

	if m.errMsg != "" {
		b.WriteString("\n")
		b.WriteString(crossStyle.Render(m.errMsg))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate  enter next  esc back  q quit"))
	return b.String()
}

func (m *Model) viewConfirmDelete() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Confirm"))
	b.WriteString("\n")
	b.WriteString(m.confirmMsg)
	b.WriteString("\n\n")
	b.WriteString(checkStyle.Render("y confirm"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(crossStyle.Render("n cancel"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(purpleStyle.Render("q quit"))
	return b.String()
}

func (m *Model) viewFiles() string {
	var b strings.Builder
	profile := m.cfg.Profiles[m.profiles[m.selectedProf]]
	b.WriteString(titleStyle.Render(fmt.Sprintf("Uplift — %s — choose files", m.profiles[m.selectedProf])))
	b.WriteString(dimStyle.Render(fmt.Sprintf("Local base: %s", profile.LocalBase)))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render(fmt.Sprintf("Remote base: %s", profile.RemoteBase)))
	b.WriteString("\n\n")

	if len(m.files) == 0 {
		b.WriteString(dimStyle.Render("No files found in local base."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc back  q quit"))
		return b.String()
	}

	visibleCount := 15
	start := m.scrollOffset
	end := start + visibleCount
	if end > len(m.files) {
		end = len(m.files)
	}

	if start > 0 {
		b.WriteString(dimStyle.Render(fmt.Sprintf("↑ %d more above", start)))
		b.WriteString("\n")
	}

	for i := start; i < end; i++ {
		f := m.files[i]
		cursor := "  "
		if i == m.cursor {
			cursor = selectStyle.Render("> ")
		}
		mark := "  "
		if m.selected[i] {
			mark = checkStyle.Render("✓ ")
		}
		line := fmt.Sprintf("%s%s%s", cursor, mark, f)
		if i == m.cursor && m.selected[i] {
			line = checkStyle.Render(fmt.Sprintf("> ✓ %s", f))
		} else if i == m.cursor {
			line = selectStyle.Render(fmt.Sprintf(">   %s", f))
		} else if m.selected[i] {
			line = checkStyle.Render(fmt.Sprintf("  ✓ %s", f))
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if end < len(m.files) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("↓ %d more below", len(m.files)-end)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓ navigate  "))
	b.WriteString(selectStyle.Render("space select"))
	b.WriteString(helpStyle.Render("  "))

	if len(m.selected) > 0 {
		b.WriteString(checkStyle.Render("enter upload"))
	} else {
		b.WriteString(helpStyle.Render("enter upload"))
	}

	b.WriteString(helpStyle.Render("  "))

	if m.batchEnabled {
		b.WriteString(checkStyle.Render("[✓]"))
		b.WriteString(helpStyle.Render(" batch"))
	} else {
		b.WriteString(crossStyle.Render("[✗]"))
		b.WriteString(dimStyle.Render(" batch"))
	}

	b.WriteString(helpStyle.Render(" (b)  "))
	b.WriteString(crossStyle.Render("esc back"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(purpleStyle.Render("q quit"))
	return b.String()
}

func (m *Model) viewUploading() string {
	return titleStyle.Render("Uploading...")
}

func (m *Model) viewSummary() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Upload complete"))
	b.WriteString(fmt.Sprintf("Uploaded: %s\n", checkStyle.Render(fmt.Sprint(m.uploaded))))
	b.WriteString(fmt.Sprintf("Skipped:  %s\n", dimStyle.Render(fmt.Sprint(m.skipped))))
	b.WriteString(fmt.Sprintf("Failed:   %s\n", crossStyle.Render(fmt.Sprint(m.failed))))

	if m.failed > 0 {
		b.WriteString("\nFailed files:\n")
		for _, r := range m.results {
			if r.Err != nil {
				b.WriteString(fmt.Sprintf("  %s\n    → %s\n    %v\n", r.Job.LocalPath, r.Job.RemotePath, r.Err))
			}
		}
	}

	if m.up != nil && m.up.LogPath() != "" {
		b.WriteString("\n")
		b.WriteString(dimStyle.Render(fmt.Sprintf("Log: %s", m.up.LogPath())))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("enter quit"))
	return b.String()
}

func (m *Model) loadFiles() {
	profile := m.cfg.Profiles[m.profiles[m.selectedProf]]
	entries, err := os.ReadDir(profile.LocalBase)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m.files = append(m.files, e.Name())
	}
	sort.Strings(m.files)
}

type remoteStatInfo struct {
	exists bool
	size   int64
	modTime time.Time
}

type remoteStatsMsg struct {
	stats map[string]remoteStatInfo
}

type uploadResultMsg struct {
	results  []uploader.Result
	uploaded int
	skipped  int
	failed   int
	err      error
}

func (m *Model) prepareUpload() tea.Cmd {
	profile := m.cfg.Profiles[m.profiles[m.selectedProf]]
	m.up = uploader.New(profile, m.writer)

	log, err := uploader.NewSessionLog(logDir())
	if err == nil {
		m.sessionLog = log
		m.up.SetSessionLog(log)
	}

	var selected []string
	for i := range m.files {
		if m.selected[i] {
			selected = append(selected, m.files[i])
		}
	}

	jobs, err := m.up.Resolve(selected)
	if err != nil {
		m.errMsg = err.Error()
		m.current = screenSummary
		m.failed = 1
		return nil
	}

	m.pendingJobs = jobs
	m.remoteStats = nil
	m.batchOffset = 0
	m.allResults = nil
	m.allUploaded = 0
	m.allSkipped = 0
	m.allFailed = 0
	m.current = screenConfirmUpload

	return m.fetchRemoteStats(jobs, 0)
}

const batchSize = 5

func (m *Model) currentBatch() []uploader.Job {
	if !m.batchEnabled {
		return m.pendingJobs
	}

	start := m.batchOffset
	end := start + batchSize
	if end > len(m.pendingJobs) {
		end = len(m.pendingJobs)
	}
	if start > len(m.pendingJobs) {
		return nil
	}
	return m.pendingJobs[start:end]
}

func (m *Model) fetchRemoteStats(jobs []uploader.Job, offset int) tea.Cmd {
	return func() tea.Msg {
		stats := make(map[string]remoteStatInfo)

		conn, err := m.up.ConnectFn()(m.cfg.Profiles[m.profiles[m.selectedProf]])
		if err != nil {
			return remoteStatsMsg{stats: stats}
		}
		defer conn.Close()

		sftpClient, err := conn.NewSFTPClient()
		if err != nil {
			return remoteStatsMsg{stats: stats}
		}
		defer sftpClient.Close()

		for _, job := range jobs {
			if info, err := sftpClient.Stat(job.RemotePath); err == nil {
				stats[job.RemotePath] = remoteStatInfo{
					exists:  true,
					size:    info.Size(),
					modTime: info.ModTime(),
				}
			} else {
				stats[job.RemotePath] = remoteStatInfo{exists: false}
			}
		}

		return remoteStatsMsg{stats: stats}
	}
}

func (m *Model) updateConfirmUpload(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "n", "N":
			m.current = screenFiles
		case "y", "Y", "enter":
			return m, m.startUpload()
		}
	}
	return m, nil
}

func (m *Model) viewConfirmUpload() string {
	var b strings.Builder
	batch := m.currentBatch()
	totalBatches := (len(m.pendingJobs) + batchSize - 1) / batchSize
	currentBatchNum := (m.batchOffset / batchSize) + 1

	b.WriteString(titleStyle.Render(fmt.Sprintf("Confirm upload (%d/%d)", currentBatchNum, totalBatches)))
	b.WriteString("\n")

	for _, job := range batch {
		b.WriteString(fmt.Sprintf("  %s\n", job.RelPath))

		b.WriteString(fmt.Sprintf("    %s", localInfoStyle.Render(job.LocalPath)))
		b.WriteString(dimStyle.Render(" → "))
		b.WriteString(remoteInfoStyle.Render(job.RemotePath))
		b.WriteString("\n")

		if info, err := os.Stat(job.LocalPath); err == nil {
			b.WriteString(fmt.Sprintf("      Size: %s", localInfoStyle.Render(formatSize(info.Size()))))
			b.WriteString(dimStyle.Render(" → "))
			if m.remoteStats == nil {
				b.WriteString(dimStyle.Render("Checking remote..."))
			} else if stat, ok := m.remoteStats[job.RemotePath]; ok && stat.exists {
				b.WriteString(remoteInfoStyle.Render(formatSize(stat.size)))
				if stat.size > info.Size() {
					b.WriteString(warnStyle.Render("  ⚠ remote is larger"))
				}
			} else {
				b.WriteString(dimStyle.Render("(does not exist)"))
			}
			b.WriteString("\n")
		}

		if info, err := os.Stat(job.LocalPath); err == nil {
			b.WriteString(fmt.Sprintf("      Modified: %s", localInfoStyle.Render(info.ModTime().Format("2006-01-02 15:04:05"))))
			b.WriteString(dimStyle.Render(" → "))
			if m.remoteStats == nil {
				b.WriteString(dimStyle.Render("Checking remote..."))
			} else if stat, ok := m.remoteStats[job.RemotePath]; ok && stat.exists {
				b.WriteString(remoteInfoStyle.Render(stat.modTime.Format("2006-01-02 15:04:05")))
			} else {
				b.WriteString(dimStyle.Render("(does not exist)"))
			}
			b.WriteString("\n")
		}

		b.WriteString("\n")
	}

	b.WriteString(checkStyle.Render("y confirm"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(crossStyle.Render("n cancel"))
	b.WriteString(helpStyle.Render("  "))
	b.WriteString(purpleStyle.Render("q quit"))
	return b.String()
}

func logDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "uplift-logs")
	}
	return filepath.Join(home, ".uplift", "logs")
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (m *Model) startUpload() tea.Cmd {
	batch := m.currentBatch()
	if len(batch) == 0 {
		m.current = screenSummary
		return nil
	}

	m.current = screenUploading

	return func() tea.Msg {
		results, err := m.up.Upload(batch, uploader.Options{
			Concurrency:  m.concurrency,
			CreateRemote: true,
			Confirm: func(job uploader.Job) (bool, error) {
				return true, nil
			},
			OverwriteCheck: func(remotePath string) (bool, error) {
				return true, nil
			},
		})
		uploaded, skipped, failed := m.up.Summarize(results)
		return uploadResultMsg{results: results, uploaded: uploaded, skipped: skipped, failed: failed, err: err}
	}
}

func Run(cfg *config.Config, configPath string) error {
	m := New(cfg, configPath)
	p := tea.NewProgram(m)
	_, err := p.Run()
	return err
}
