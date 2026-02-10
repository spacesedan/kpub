package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spacesedan/kpub/internal/config"
	"github.com/spacesedan/kpub/internal/setup"
)

type addChatPhase int

const (
	chatPhaseInput   addChatPhase = iota
	chatPhaseConfirm
	chatPhaseDone
)

// AddChatModel is the Bubbletea model for the add-chat command.
type AddChatModel struct {
	dataDir string
	cfg     *config.Config

	phase    addChatPhase
	input    textinput.Model
	inputErr string

	// Collected value
	handle string

	// Final state
	done    bool
	aborted bool
	err     error
	result  string
}

// NewAddChatModel creates a new add-chat model, loading the existing config.
func NewAddChatModel(dataDir string) AddChatModel {
	configPath := filepath.Join(dataDir, "config.yaml")
	cfg, loadErr := config.Load(configPath)

	m := AddChatModel{
		dataDir: dataDir,
		cfg:     cfg,
		phase:   chatPhaseInput,
	}

	if loadErr != nil {
		m.done = true
		m.err = fmt.Errorf("loading config: %w", loadErr)
		return m
	}

	m.initInput()
	return m
}

func (m *AddChatModel) initInput() {
	handle := textinput.New()
	handle.Placeholder = "@ebook-bot"
	handle.Prompt = Prompt.Render("  Handle: ")
	handle.Focus()

	m.input = handle
}

func (m AddChatModel) Init() tea.Cmd {
	if m.done {
		return tea.Quit
	}
	return textinput.Blink
}

func (m AddChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done || m.aborted {
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.String() == "ctrl+c" {
		m.aborted = true
		return m, tea.Quit
	}

	switch m.phase {
	case chatPhaseInput:
		return m.updateInput(msg)
	case chatPhaseConfirm:
		return m.updateConfirm(msg)
	}

	return m, nil
}

func (m AddChatModel) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.input.Value())
		if val == "" {
			m.inputErr = "Value cannot be empty"
			return m, nil
		}
		if !strings.HasPrefix(val, "@") {
			m.inputErr = "Handle must start with @"
			return m, nil
		}

		// Validate duplicate handle
		for _, chat := range m.cfg.Chats {
			if chat.Handle == val {
				m.inputErr = fmt.Sprintf("Chat %q already exists", val)
				return m, nil
			}
		}

		m.handle = val
		m.inputErr = ""
		m.phase = chatPhaseConfirm
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m AddChatModel) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "y", "Y", "enter":
			return m.save()
		case "n", "N":
			m.aborted = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m AddChatModel) save() (tea.Model, tea.Cmd) {
	m.cfg.Chats = append(m.cfg.Chats, config.ChatConfig{
		Handle: m.handle,
	})

	if err := setup.WriteConfig(m.dataDir, m.cfg); err != nil {
		m.err = fmt.Errorf("writing config: %w", err)
		m.done = true
		return m, tea.Quit
	}

	m.done = true
	m.result = Success.Render(fmt.Sprintf("Chat %q added!", m.handle)) + "\n\n" +
		"  " + Dim.Render(fmt.Sprintf("Total chats: %d", len(m.cfg.Chats)))
	return m, tea.Quit
}

// Err returns any error that occurred.
func (m AddChatModel) Err() error {
	return m.err
}

func (m AddChatModel) View() string {
	if m.aborted {
		return "\n" + Warning.Render("  Cancelled.") + "\n\n"
	}
	if m.done {
		if m.err != nil {
			return "\n" + Error.Render("  Error: "+m.err.Error()) + "\n\n"
		}
		return "\n  " + m.result + "\n\n"
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + Title.Render("Add a new chat to monitor") + "\n\n")

	// Show existing chats
	if len(m.cfg.Chats) > 0 {
		b.WriteString("  " + Dim.Render(fmt.Sprintf("Existing chats: %d", len(m.cfg.Chats))) + "\n")
		for _, chat := range m.cfg.Chats {
			b.WriteString("    " + Dim.Render("- "+chat.Handle) + "\n")
		}
		b.WriteString("\n")
	}

	switch m.phase {
	case chatPhaseInput:
		b.WriteString("  " + m.input.View() + "\n")
		if m.inputErr != "" {
			b.WriteString("  " + Warning.Render("  "+m.inputErr) + "\n")
		}
	case chatPhaseConfirm:
		b.WriteString("  " + Highlight.Render("Summary:") + "\n")
		b.WriteString(fmt.Sprintf("    Handle: %s\n", m.handle))
		b.WriteString("\n")
		b.WriteString("  " + Prompt.Render("Add this chat? [Y/n] "))
	}

	return b.String()
}
