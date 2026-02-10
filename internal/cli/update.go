package cli

import (
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spacesedan/kpub/internal/dockerutil"
)

type updatePhase int

const (
	updatePulling    updatePhase = iota
	updateRestarting
	updateDone
)

type updateStepDoneMsg struct{ err error }

// updateOutputMsg carries a single line of docker output (update command).
type updateOutputMsg string

// UpdateModel is the Bubbletea model for the `update` command.
type UpdateModel struct {
	dataDir  string
	restart  bool
	image    string
	phase    updatePhase
	spinner  spinner.Model
	outputCh chan string
	status   string
	err      error
	done     bool
}

// NewUpdateModel creates a new update command model.
func NewUpdateModel(dataDir string, restart bool, image string) UpdateModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Highlight

	return UpdateModel{
		dataDir:  dataDir,
		restart:  restart,
		image:    image,
		phase:    updatePulling,
		spinner:  s,
		outputCh: make(chan string, 128),
	}
}

func (m UpdateModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.pullImage(), m.listenOutput())
}

func (m UpdateModel) listenOutput() tea.Cmd {
	ch := m.outputCh
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return updateOutputMsg(line)
	}
}

func (m UpdateModel) pullImage() tea.Cmd {
	ch := m.outputCh
	image := m.image
	return func() tea.Msg {
		err := dockerutil.PullImage(image, ch)
		return updateStepDoneMsg{err: err}
	}
}

func (m UpdateModel) restartContainer() tea.Cmd {
	image := m.image
	return func() tea.Msg {
		if err := dockerutil.RemoveContainer("kpub"); err != nil {
			return updateStepDoneMsg{err: err}
		}
		err := dockerutil.RunContainer("kpub", image, m.dataDir, true)
		return updateStepDoneMsg{err: err}
	}
}

func (m UpdateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case updateOutputMsg:
		if clean, ok := FilterDockerLine(string(msg)); ok {
			m.status = clean
		}
		return m, m.listenOutput()
	case updateStepDoneMsg:
		m.status = ""
		if msg.err != nil {
			m.err = msg.err
			m.done = true
			return m, tea.Quit
		}
		switch m.phase {
		case updatePulling:
			if m.restart {
				m.phase = updateRestarting
				return m, m.restartContainer()
			}
			m.phase = updateDone
			m.done = true
			return m, tea.Quit
		case updateRestarting:
			m.phase = updateDone
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

func (m UpdateModel) View() string {
	if m.done {
		if m.err != nil {
			return "\n" + Error.Render("  Error: "+m.err.Error()) + "\n\n"
		}
		msg := Success.Render("  Update complete!")
		if m.restart {
			msg += "\n  " + Dim.Render("Container restarted. Use 'docker logs -f kpub' to view logs.")
		} else {
			msg += "\n  " + Dim.Render("Run 'kpub run' to start the updated container.")
		}
		return "\n" + msg + "\n\n"
	}

	var b strings.Builder
	b.WriteString("\n")

	type viewStep struct {
		label string
		phase updatePhase
	}

	steps := []viewStep{
		{"Pulling " + m.image + "...", updatePulling},
	}
	if m.restart {
		steps = append(steps, viewStep{"Restarting container...", updateRestarting})
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

// Err returns any error that occurred.
func (m UpdateModel) Err() error {
	return m.err
}
