package cli

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spacesedan/kpub/internal/dockerutil"
)

type runPhase int

const (
	runRemoving runPhase = iota
	runPulling
	runStarting
	runDone
)

type runStepDoneMsg struct{ err error }

// dockerOutputMsg carries a single line of docker output.
type dockerOutputMsg string

// RunModel is the Bubbletea model for the `run` command.
type RunModel struct {
	dataDir  string
	detach   bool
	image    string
	phase    runPhase
	spinner  spinner.Model
	outputCh chan string // receives streaming docker output
	status   string     // latest output line
	err      error
	done     bool
}

// NewRunModel creates a new run command model.
func NewRunModel(dataDir string, detach bool, image string) RunModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Highlight

	return RunModel{
		dataDir:  dataDir,
		detach:   detach,
		image:    image,
		phase:    runRemoving,
		spinner:  s,
		outputCh: make(chan string, 128),
	}
}

func (m RunModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.removeContainer())
}

func (m RunModel) listenOutput() tea.Cmd {
	ch := m.outputCh
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return dockerOutputMsg(line)
	}
}

func (m RunModel) removeContainer() tea.Cmd {
	return func() tea.Msg {
		err := dockerutil.RemoveContainer("kpub")
		return runStepDoneMsg{err: err}
	}
}

func (m RunModel) pullImage() tea.Cmd {
	ch := m.outputCh
	image := m.image
	return func() tea.Msg {
		err := dockerutil.PullImage(image, ch)
		return runStepDoneMsg{err: err}
	}
}

func (m RunModel) startContainer() tea.Cmd {
	image := m.image
	return func() tea.Msg {
		err := dockerutil.RunContainer("kpub", image, m.dataDir, m.detach)
		return runStepDoneMsg{err: err}
	}
}

func (m RunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case dockerOutputMsg:
		if clean, ok := FilterDockerLine(string(msg)); ok {
			m.status = clean
		}
		return m, m.listenOutput()
	case runStepDoneMsg:
		m.status = ""
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		switch m.phase {
		case runRemoving:
			m.phase = runPulling
			return m, tea.Batch(m.pullImage(), m.listenOutput())
		case runPulling:
			if !m.detach {
				m.phase = runStarting
				m.done = true
				return m, tea.Quit
			}
			m.phase = runStarting
			return m, m.startContainer()
		case runStarting:
			m.phase = runDone
			m.done = true
			return m, tea.Quit
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m RunModel) View() string {
	if m.done {
		if m.err != nil {
			return "\n" + Error.Render("  Error: "+m.err.Error()) + "\n\n"
		}
		if m.detach {
			return "\n" + Success.Render("  Container started in background.") + "\n" +
				"  " + Dim.Render("Use 'docker logs -f kpub' to view logs.") + "\n\n"
		}
		return ""
	}

	var b strings.Builder
	b.WriteString("\n")

	steps := []struct {
		label string
		phase runPhase
	}{
		{"Removing old container...", runRemoving},
		{"Pulling image...", runPulling},
		{"Starting container...", runStarting},
	}

	for _, s := range steps {
		if m.phase > s.phase {
			b.WriteString("  " + Success.Render("\u2713 "+s.label) + "\n")
		} else if m.phase == s.phase {
			b.WriteString("  " + m.spinner.View() + " " + s.label + "\n")
		} else {
			b.WriteString("  " + Dim.Render("  "+s.label) + "\n")
		}
	}

	if m.status != "" {
		b.WriteString("\n  " + Dim.Render(TruncateLine(m.status, 72)) + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// NeedsForegroundRun returns true if the model completed the pull phase
// and needs a foreground docker run (i.e., not detached).
func (m RunModel) NeedsForegroundRun() bool {
	return m.done && m.err == nil && !m.detach && m.phase == runStarting
}

// RunForeground executes docker run in the foreground, taking over the terminal.
func RunForeground(image, dataDir string) error {
	return dockerutil.RunContainer("kpub", image, dataDir, false)
}

// Err returns any error that occurred.
func (m RunModel) Err() error {
	return m.err
}
