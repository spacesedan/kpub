package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/spacesedan/kpub/internal/setup"
)

// wizardStep enumerates the setup wizard steps.
type wizardStep int

const (
	stepTelegram    wizardStep = iota
	stepDropboxApp  wizardStep = iota
	stepDropboxAuth wizardStep = iota
	stepChats       wizardStep = iota
	stepReview      wizardStep = iota
)

const totalSteps = 5

const banner = ` _                _
| | ___ __  _   _| |__
| |/ / '_ \| | | | '_ \
|   <| |_) | |_| | |_) |
|_|\_\ .__/ \__,_|_.__/
     |_|`

// chatEntry holds one chat's handle during setup.
type chatEntry struct {
	handle string
}

// SetupModel is the Bubbletea model for the setup wizard.
type SetupModel struct {
	dataDir string
	step    wizardStep

	// Text inputs (reused across steps)
	inputs    []textinput.Model
	inputIdx  int
	inputErr  string

	// Spinner for async operations
	spinner spinner.Model

	// Wizard state
	appID            int
	appHash          string
	dropboxAppKey    string
	dropboxAppSecret string
	tokens           *setup.DropboxTokens
	chats            []chatEntry

	// Step-specific state
	exchanging      bool // true while exchanging dropbox code
	exchangeErr     string
	browserOpened   bool // true after we've tried to open the browser
	addingChat      bool // true when entering a new chat
	confirmingChat  bool // asking "add another?"
	confirmSave     bool // on review step, waiting for y/n

	// Final state
	done    bool
	aborted bool
	err     error
	result  string // success message
}

// tokenExchangeMsg is sent when the Dropbox token exchange completes.
type tokenExchangeMsg struct {
	tokens *setup.DropboxTokens
	err    error
}

// browserOpenedMsg is sent after attempting to open the browser.
type browserOpenedMsg struct{}

func openBrowserCmd(url string) tea.Cmd {
	return func() tea.Msg {
		_ = setup.OpenBrowser(url)
		return browserOpenedMsg{}
	}
}

// NewSetupModel creates a new setup wizard model.
func NewSetupModel(dataDir string) SetupModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = Highlight

	m := SetupModel{
		dataDir: dataDir,
		step:    stepTelegram,
		spinner: s,
	}
	m.initStepInputs()
	return m
}

func (m *SetupModel) initStepInputs() {
	switch m.step {
	case stepTelegram:
		appID := textinput.New()
		appID.Placeholder = "12345678"
		appID.Prompt = Prompt.Render("  App ID: ")
		appID.Focus()

		appHash := textinput.New()
		appHash.Placeholder = "0123456789abcdef..."
		appHash.Prompt = Prompt.Render("  App Hash: ")

		m.inputs = []textinput.Model{appID, appHash}
		m.inputIdx = 0

	case stepDropboxApp:
		appKey := textinput.New()
		appKey.Placeholder = "your-app-key"
		appKey.Prompt = Prompt.Render("  App Key: ")
		appKey.Focus()

		appSecret := textinput.New()
		appSecret.Placeholder = "your-app-secret"
		appSecret.Prompt = Prompt.Render("  App Secret: ")
		appSecret.EchoMode = textinput.EchoPassword

		m.inputs = []textinput.Model{appKey, appSecret}
		m.inputIdx = 0

	case stepDropboxAuth:
		code := textinput.New()
		code.Placeholder = "paste authorization code"
		code.Prompt = Prompt.Render("  Auth Code: ")
		code.Focus()

		m.inputs = []textinput.Model{code}
		m.inputIdx = 0
		m.exchanging = false
		m.exchangeErr = ""
		m.browserOpened = false

	case stepChats:
		m.chats = nil
		m.addingChat = true
		m.confirmingChat = false
		m.initChatInput()

	case stepReview:
		m.confirmSave = true
		m.inputs = nil
		m.inputIdx = 0
	}

	m.inputErr = ""
}

func (m *SetupModel) initChatInput() {
	handle := textinput.New()
	handle.Placeholder = "@ebook-bot"
	handle.Prompt = Prompt.Render("  Handle: ")
	handle.Focus()

	m.inputs = []textinput.Model{handle}
	m.inputIdx = 0
}

func (m SetupModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, textinput.Blink)
}

func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.aborted = true
			return m, tea.Quit
		case "esc":
			return m.goBack()
		}
	case tokenExchangeMsg:
		m.exchanging = false
		if msg.err != nil {
			m.exchangeErr = msg.err.Error()
			// Re-enable input
			m.inputs[0].SetValue("")
			m.inputs[0].Focus()
			return m, textinput.Blink
		}
		m.tokens = msg.tokens
		m.step = stepChats
		m.initStepInputs()
		return m, textinput.Blink
	case browserOpenedMsg:
		m.browserOpened = true
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.done || m.aborted {
		return m, nil
	}

	switch m.step {
	case stepTelegram:
		return m.updateTelegram(msg)
	case stepDropboxApp:
		return m.updateDropboxApp(msg)
	case stepDropboxAuth:
		return m.updateDropboxAuth(msg)
	case stepChats:
		return m.updateChats(msg)
	case stepReview:
		return m.updateReview(msg)
	}

	return m, nil
}

func (m SetupModel) goBack() (tea.Model, tea.Cmd) {
	if m.step > stepTelegram {
		m.step--
		m.initStepInputs()
		cmds := []tea.Cmd{textinput.Blink}
		if m.step == stepDropboxAuth {
			cmds = append(cmds, openBrowserCmd(setup.DropboxAuthURL(m.dropboxAppKey)))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// --- Step update handlers ---

func (m SetupModel) updateTelegram(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.inputs[m.inputIdx].Value())
		if val == "" {
			m.inputErr = "Value cannot be empty"
			return m, nil
		}
		if strings.EqualFold(val, "back") {
			return m.goBack()
		}

		if m.inputIdx == 0 {
			n, err := strconv.Atoi(val)
			if err != nil {
				m.inputErr = "Please enter a valid number"
				return m, nil
			}
			m.appID = n
			m.inputErr = ""
			m.inputs[m.inputIdx].Blur()
			m.inputIdx = 1
			m.inputs[m.inputIdx].Focus()
			return m, textinput.Blink
		}

		// Second field (App Hash)
		m.appHash = val
		m.inputErr = ""
		m.step = stepDropboxApp
		m.initStepInputs()
		return m, textinput.Blink
	}

	return m.updateActiveInput(msg)
}

func (m SetupModel) updateDropboxApp(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.inputs[m.inputIdx].Value())
		if val == "" {
			m.inputErr = "Value cannot be empty"
			return m, nil
		}
		if strings.EqualFold(val, "back") {
			return m.goBack()
		}

		if m.inputIdx == 0 {
			m.dropboxAppKey = val
			m.inputErr = ""
			m.inputs[m.inputIdx].Blur()
			m.inputIdx = 1
			m.inputs[m.inputIdx].Focus()
			return m, textinput.Blink
		}

		m.dropboxAppSecret = val
		m.inputErr = ""
		m.step = stepDropboxAuth
		m.initStepInputs()
		authURL := setup.DropboxAuthURL(m.dropboxAppKey)
		return m, tea.Batch(textinput.Blink, openBrowserCmd(authURL))
	}

	return m.updateActiveInput(msg)
}

func (m SetupModel) updateDropboxAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.exchanging {
		return m, nil
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		val := strings.TrimSpace(m.inputs[0].Value())
		if val == "" {
			m.inputErr = "Value cannot be empty"
			return m, nil
		}
		if strings.EqualFold(val, "back") {
			return m.goBack()
		}

		m.exchanging = true
		m.exchangeErr = ""
		m.inputErr = ""
		appKey := m.dropboxAppKey
		appSecret := m.dropboxAppSecret
		return m, func() tea.Msg {
			tokens, err := setup.ExchangeDropboxCode(appKey, appSecret, val)
			return tokenExchangeMsg{tokens: tokens, err: err}
		}
	}

	return m.updateActiveInput(msg)
}

func (m SetupModel) updateChats(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		if m.confirmingChat {
			switch key.String() {
			case "y", "Y":
				m.confirmingChat = false
				m.addingChat = true
				m.initChatInput()
				return m, textinput.Blink
			case "n", "N", "enter":
				m.confirmingChat = false
				m.addingChat = false
				m.step = stepReview
				m.initStepInputs()
				return m, nil
			}
			return m, nil
		}

		if key.Type == tea.KeyEnter {
			val := strings.TrimSpace(m.inputs[0].Value())
			if val == "" {
				m.inputErr = "Value cannot be empty"
				return m, nil
			}
			if strings.EqualFold(val, "back") {
				return m.goBack()
			}
			if !strings.HasPrefix(val, "@") {
				m.inputErr = "Handle must start with @"
				return m, nil
			}

			m.chats = append(m.chats, chatEntry{handle: val})
			m.inputErr = ""
			m.confirmingChat = true
			return m, nil
		}
	}

	return m.updateActiveInput(msg)
}

func (m SetupModel) updateReview(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "y", "Y", "enter":
			return m.saveConfig()
		case "n", "N":
			m.done = true
			m.result = Warning.Render("Aborted. No files were written.")
			return m, tea.Quit
		case "b", "B":
			return m.goBack()
		}
	}
	return m, nil
}

func (m SetupModel) saveConfig() (tea.Model, tea.Cmd) {
	cfg := setup.BuildConfig(m.appID, m.appHash, m.dropboxAppKey, m.dropboxAppSecret, m.chatsToSetupChats())

	if err := setup.WriteConfig(m.dataDir, cfg); err != nil {
		m.err = fmt.Errorf("writing config: %w", err)
		m.done = true
		return m, tea.Quit
	}
	if err := setup.WriteDropboxTokens(m.dataDir, m.tokens); err != nil {
		m.err = fmt.Errorf("writing dropbox tokens: %w", err)
		m.done = true
		return m, tea.Quit
	}

	m.done = true
	configPath := m.dataDir + "/config.yaml"
	tokenPath := m.dataDir + "/dropbox.json"
	m.result = Success.Render("All done!") + "\n\n" +
		"  Files written:\n" +
		"    " + Highlight.Render(configPath) + "\n" +
		"    " + Highlight.Render(tokenPath) + "\n\n" +
		"  " + Title.Render("Next steps:") + "\n" +
		"    1. " + Highlight.Render("kpub run") + "\n\n" +
		"  Happy reading!"
	return m, tea.Quit
}

func (m SetupModel) chatsToSetupChats() []setup.ChatInput {
	out := make([]setup.ChatInput, len(m.chats))
	for i, c := range m.chats {
		out[i] = setup.ChatInput{Handle: c.handle}
	}
	return out
}

func (m SetupModel) updateActiveInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.inputIdx < len(m.inputs) {
		var cmd tea.Cmd
		m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
		return m, cmd
	}
	return m, nil
}

// --- View ---

func (m SetupModel) View() string {
	if m.aborted {
		return "\n" + Warning.Render("  Setup cancelled.") + "\n\n"
	}
	if m.done {
		if m.err != nil {
			return "\n" + Error.Render("  Error: "+m.err.Error()) + "\n\n"
		}
		return "\n  " + m.result + "\n\n"
	}

	var b strings.Builder

	// Banner
	b.WriteString("\n")
	for _, line := range strings.Split(banner, "\n") {
		b.WriteString("  " + Title.Render(line) + "\n")
	}
	b.WriteString("  " + Dim.Render("~ kpub setup wizard ~") + "\n\n")
	b.WriteString("  " + Title.Render("Let's get your ebook pipeline set up!") + "\n")
	b.WriteString("  Files will be saved to " + Highlight.Render(m.dataDir+"/") + "\n")
	b.WriteString("  " + Dim.Render("Type \"back\" or press Esc to go to the previous step.") + "\n\n")

	// Progress bar
	filled := int(m.step) + 1
	bar := strings.Repeat("#", filled) + strings.Repeat("-", totalSteps-filled)
	b.WriteString("  " + Dim.Render(fmt.Sprintf("[%s] Step %d/%d", bar, filled, totalSteps)) + "\n")

	// Step title + content
	switch m.step {
	case stepTelegram:
		b.WriteString("  " + Title.Render("\u2708\ufe0f  Telegram credentials") + "\n\n")
		telegramLink := Link("https://my.telegram.org/apps", Highlight.Render("my.telegram.org/apps"))
		b.WriteString("  Head over to " + telegramLink + " and grab your\n")
		b.WriteString("  API credentials. You'll need the numeric App ID and the App Hash.\n\n")
		b.WriteString(m.renderInputs())

	case stepDropboxApp:
		b.WriteString("  " + Title.Render("\U0001f4e6 Dropbox app credentials") + "\n\n")
		dropboxLink := Link("https://www.dropbox.com/developers/apps", Highlight.Render("dropbox.com/developers/apps"))
		b.WriteString("  Create a Dropbox app at " + dropboxLink + "\n")
		b.WriteString("  (Full Dropbox access, no redirect URI needed)\n\n")
		b.WriteString(m.renderInputs())

	case stepDropboxAuth:
		b.WriteString("  " + Title.Render("\U0001f511 Dropbox authorization") + "\n\n")
		authURL := setup.DropboxAuthURL(m.dropboxAppKey)
		authLink := Link(authURL, Highlight.Render(authURL))
		if m.browserOpened {
			b.WriteString("  Opening your browser now...\n")
			b.WriteString("  " + Dim.Render("If it didn't open, click or copy this URL:") + "\n")
		} else {
			b.WriteString("  Open this URL in your browser:\n")
		}
		b.WriteString("  " + authLink + "\n\n")
		if m.exchanging {
			b.WriteString("  " + m.spinner.View() + " Exchanging code for tokens...\n")
		} else {
			if m.exchangeErr != "" {
				b.WriteString("  " + Error.Render("Authorization failed: "+m.exchangeErr) + "\n")
				b.WriteString("  " + Dim.Render("Try again with a new code, or type \"back\" to fix your credentials.") + "\n\n")
			}
			b.WriteString(m.renderInputs())
		}

	case stepChats:
		b.WriteString("  " + Title.Render("\U0001f4ac Chat configuration") + "\n\n")
		b.WriteString("  Enter the handles of the chats you want to monitor for ebook files.\n")
		b.WriteString("  This can be bots, groups, or channels (e.g. @ebook-bot, @bookgroup).\n")
		b.WriteString("  You need at least one, but you can add as many as you like.\n\n")
		// Show already-added chats
		for i, chat := range m.chats {
			b.WriteString("  " + Success.Render(fmt.Sprintf("  Chat #%d: %s", i+1, chat.handle)) + "\n")
		}
		if len(m.chats) > 0 {
			b.WriteString("\n")
		}
		if m.confirmingChat {
			b.WriteString("  " + Success.Render(fmt.Sprintf("Chat %q added.", m.chats[len(m.chats)-1].handle)) + "\n\n")
			b.WriteString("  " + Prompt.Render("Add another chat? [y/N] "))
		} else if m.addingChat {
			num := len(m.chats) + 1
			b.WriteString("  " + Highlight.Render(fmt.Sprintf("--- Chat #%d ---", num)) + "\n\n")
			b.WriteString(m.renderInputs())
		}

	case stepReview:
		b.WriteString("  " + Title.Render("\u2705 Review and save") + "\n\n")
		b.WriteString("  Here's what we've got:\n\n")
		b.WriteString("  " + Title.Render("\u2708\ufe0f  Telegram") + "\n")
		b.WriteString(fmt.Sprintf("    App ID:        %d\n", m.appID))
		b.WriteString(fmt.Sprintf("    App Hash:      %s\n", setup.Mask(m.appHash)))
		b.WriteString("\n")
		b.WriteString("  " + Title.Render("\U0001f4e6 Dropbox") + "\n")
		b.WriteString(fmt.Sprintf("    App Key:       %s\n", m.dropboxAppKey))
		b.WriteString(fmt.Sprintf("    App Secret:    %s\n", setup.Mask(m.dropboxAppSecret)))
		b.WriteString(fmt.Sprintf("    Access Token:  %s\n", setup.Mask(m.tokens.AccessToken)))
		b.WriteString("\n")
		b.WriteString("  " + Title.Render("\U0001f4ac Chats") + "\n")
		for _, chat := range m.chats {
			b.WriteString(fmt.Sprintf("    %s\n", Highlight.Render(chat.handle)))
		}
		b.WriteString("\n")
		if m.confirmSave {
			b.WriteString("  " + Prompt.Render("Save configuration? [Y/n] "))
		}
	}

	return b.String()
}

func (m SetupModel) renderInputs() string {
	var b strings.Builder
	for i, input := range m.inputs {
		if i < m.inputIdx {
			// Already filled — show with a check mark
			b.WriteString("  " + Success.Render("  "+input.Prompt) + Dim.Render(input.Value()) + "\n")
		} else if i == m.inputIdx {
			b.WriteString("  " + input.View() + "\n")
		}
	}
	if m.inputErr != "" {
		b.WriteString("  " + Warning.Render("  "+m.inputErr) + "\n")
	}
	return b.String()
}
