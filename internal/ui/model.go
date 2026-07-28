package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	git "github.com/go-git/go-git/v5"
	"github.com/lpmurray16/github-tui-go/internal/githubauth"
	"github.com/lpmurray16/github-tui-go/internal/gitops"
)

type mode int

const (
	modeNormal mode = iota
	modeCommit
	modeBranchName
	modeStash
	modeCheckout
	modeDiscardFile
	modeDiscardAll
)

var (
	orange      = lipgloss.Color("#ff6b1a")
	cyan        = lipgloss.Color("#00ffff")
	green       = lipgloss.Color("#44d17a")
	dim         = lipgloss.Color("#737373")
	red         = lipgloss.Color("#ff4057")
	panelStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#333333")).Padding(0, 1)
	navbarStyle = lipgloss.NewStyle().Background(lipgloss.Color("#0a0a0a")).Padding(0, 1)
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(orange)
	accentStyle = lipgloss.NewStyle().Bold(true).Foreground(cyan)
	onlineStyle = lipgloss.NewStyle().Bold(true).Foreground(green)
	errorStyle  = lipgloss.NewStyle().Foreground(red)
	dimStyle    = lipgloss.NewStyle().Foreground(dim)
)

type fileItem struct{ file gitops.FileStatus }

func (i fileItem) Title() string       { return i.file.Path }
func (i fileItem) Description() string { return statusLabel(i.file) }
func (i fileItem) FilterValue() string { return i.file.Path }

type branchItem string

func (i branchItem) Title() string       { return string(i) }
func (i branchItem) Description() string { return "local branch" }
func (i branchItem) FilterValue() string { return string(i) }

type startupMsg struct {
	repo     *gitops.Repository
	files    []gitops.FileStatus
	branches []string
	branch   string
	account  githubauth.Account
	repoErr  error
	authErr  error
}

type operationMsg struct {
	label string
	err   error
}

type authLoginMsg struct{ err error }

type Model struct {
	width, height int
	mode          mode
	busy          bool
	repo          *gitops.Repository
	repoErr       error
	authErr       error
	account       githubauth.Account
	branch        string
	branches      []string
	status        list.Model
	branchList    list.Model
	log           viewport.Model
	input         textinput.Model
	spinner       spinner.Model
	message       string
	showHelp      bool
}

func New() Model {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(cyan).BorderLeftForeground(orange)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#c8c8c8")).BorderLeftForeground(orange)

	status := list.New(nil, delegate, 46, 18)
	status.Title = "Working tree"
	status.SetShowHelp(false)
	status.SetFilteringEnabled(true)
	status.Styles.Title = titleStyle

	branches := list.New(nil, delegate, 46, 18)
	branches.Title = "Checkout branch"
	branches.SetShowHelp(false)
	branches.Styles.Title = titleStyle

	input := textinput.New()
	input.CharLimit = 200
	input.Width = 60
	input.PromptStyle = accentStyle
	input.Cursor.Style = accentStyle

	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = accentStyle

	log := viewport.New(50, 18)
	log.SetContent("Starting github-tui-go…")

	return Model{status: status, branchList: branches, input: input, spinner: spin, log: log, message: "Loading repository and GitHub account…"}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(), m.spinner.Tick)
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg {
		result := startupMsg{}
		cwd, err := os.Getwd()
		if err != nil {
			result.repoErr = err
		} else {
			result.repo, result.repoErr = gitops.Open(cwd)
		}
		if result.repoErr == nil {
			result.files, result.repoErr = result.repo.Status()
			if result.repoErr == nil {
				result.branch, result.repoErr = result.repo.CurrentBranch()
			}
			if result.repoErr == nil {
				result.branches, result.repoErr = result.repo.Branches()
			}
		}
		result.account, result.authErr = githubauth.Status()
		return result
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case startupMsg:
		m.busy = false
		m.repo, m.repoErr = msg.repo, msg.repoErr
		m.account, m.authErr = msg.account, msg.authErr
		m.branch, m.branches = msg.branch, msg.branches
		m.setFiles(msg.files)
		m.setBranches(msg.branches)
		m.updateLog()
	case operationMsg:
		m.busy = false
		if msg.err != nil {
			m.message = "Error: " + msg.err.Error()
		} else {
			m.message = msg.label
		}
		m.mode = modeNormal
		return m, refreshCmd()
	case authLoginMsg:
		m.busy = false
		if msg.err != nil {
			m.message = "GitHub login failed: " + msg.err.Error()
		} else {
			m.message = "GitHub login completed"
		}
		return m, refreshCmd()
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	}

	if key, ok := msg.(tea.KeyMsg); ok {
		if key.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.busy {
			return m, tea.Batch(cmds...)
		}
		if m.mode != modeNormal {
			return m.updateModal(key, cmds)
		}
		switch key.String() {
		case "q":
			return m, tea.Quit
		case "?":
			m.showHelp = !m.showHelp
			m.updateLog()
		case "r":
			m.busy, m.message = true, "Refreshing…"
			cmds = append(cmds, refreshCmd())
		case "g":
			m.busy, m.message = true, "Opening GitHub CLI login…"
			cmd := exec.Command("gh", "auth", "login")
			cmds = append(cmds, tea.ExecProcess(cmd, func(err error) tea.Msg { return authLoginMsg{err: err} }))
		case "c":
			if m.requireRepo() {
				cmds = append(cmds, m.openInput(modeCommit, "Commit message", "Describe this commit…", ""))
			}
		case "p":
			if m.requireRepo() {
				m.busy, m.message = true, "Pushing "+m.branch+"…"
				cmds = append(cmds, operationCmd("Pushed "+m.branch+" to origin", m.repo.Push))
			}
		case "P":
			if m.requireRepo() {
				m.busy, m.message = true, "Opening GitHub pull request form…"
				cmds = append(cmds, operationCmd("Opened pull request form in your browser", func() error {
					return githubauth.OpenPRForm(m.repo.Root, m.branch)
				}))
			}
		case "b":
			if m.requireRepo() {
				cmds = append(cmds, m.openInput(modeBranchName, "New branch from master", "feature/my-branch", ""))
			}
		case "u":
			if m.requireRepo() {
				m.busy, m.message = true, "Fetching and merging origin/master…"
				cmds = append(cmds, operationCmd("Current branch updated with origin/master", m.repo.UpdateFromMaster))
			}
		case "s":
			if m.requireRepo() {
				cmds = append(cmds, m.openInput(modeStash, "Stash message", "Optional description", ""))
			}
		case "S":
			if m.requireRepo() {
				m.busy, m.message = true, "Popping latest stash…"
				cmds = append(cmds, operationCmd("Latest stash applied", m.repo.PopStash))
			}
		case "o":
			if m.requireRepo() {
				m.mode = modeCheckout
			}
		case "x":
			if item, ok := m.status.SelectedItem().(fileItem); ok && m.requireRepo() {
				m.mode = modeDiscardFile
				m.message = fmt.Sprintf("Discard ALL staged and unstaged changes to %s? [y/N]", item.file.Path)
			}
		case "X":
			if m.requireRepo() {
				m.mode = modeDiscardAll
				m.message = "Permanently discard ALL tracked and untracked changes? [y/N]"
			}
		}
	}

	if m.mode == modeNormal {
		var cmd tea.Cmd
		m.status, cmd = m.status.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.mode == modeCheckout {
		var cmd tea.Cmd
		m.branchList, cmd = m.branchList.Update(msg)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) updateModal(key tea.KeyMsg, cmds []tea.Cmd) (tea.Model, tea.Cmd) {
	if key.String() == "esc" {
		m.mode = modeNormal
		m.input.Blur()
		m.message = "Cancelled"
		return m, tea.Batch(cmds...)
	}

	switch m.mode {
	case modeCommit:
		if key.String() == "enter" {
			message := m.input.Value()
			m.input.Blur()
			m.busy, m.message = true, "Committing all changes…"
			login := m.account.Login
			cmds = append(cmds, operationCmd("Commit created", func() error {
				_, err := m.repo.CommitAll(message, login)
				return err
			}))
			return m, tea.Batch(cmds...)
		}
	case modeBranchName:
		if key.String() == "enter" {
			name := strings.TrimSpace(m.input.Value())
			m.input.Blur()
			m.busy, m.message = true, fmt.Sprintf("Creating %s from master…", name)
			cmds = append(cmds, operationCmd(fmt.Sprintf("Created %s from the latest master", name), func() error { return m.repo.CreateBranchFromMaster(name) }))
			return m, tea.Batch(cmds...)
		}
	case modeStash:
		if key.String() == "enter" {
			message := m.input.Value()
			m.input.Blur()
			m.busy, m.message = true, "Stashing tracked and untracked changes…"
			cmds = append(cmds, operationCmd("Changes stashed", func() error { return m.repo.Stash(message) }))
			return m, tea.Batch(cmds...)
		}
	case modeCheckout:
		if key.String() == "enter" {
			if item, ok := m.branchList.SelectedItem().(branchItem); ok {
				branch := string(item)
				m.busy, m.message = true, "Checking out "+branch+"…"
				cmds = append(cmds, operationCmd("Checked out "+branch, func() error { return m.repo.Checkout(branch) }))
				return m, tea.Batch(cmds...)
			}
		}
		var cmd tea.Cmd
		m.branchList, cmd = m.branchList.Update(key)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
	case modeDiscardFile:
		if strings.EqualFold(key.String(), "y") {
			if item, ok := m.status.SelectedItem().(fileItem); ok {
				path := item.file.Path
				m.busy, m.message = true, "Discarding "+path+"…"
				cmds = append(cmds, operationCmd("Discarded changes to "+path, func() error { return m.repo.DiscardFile(path) }))
				return m, tea.Batch(cmds...)
			}
		}
		m.mode, m.message = modeNormal, "Discard cancelled"
		return m, tea.Batch(cmds...)
	case modeDiscardAll:
		if strings.EqualFold(key.String(), "y") {
			m.busy, m.message = true, "Discarding all changes…"
			cmds = append(cmds, operationCmd("Discarded all changes", m.repo.DiscardAll))
			return m, tea.Batch(cmds...)
		}
		m.mode, m.message = modeNormal, "Discard cancelled"
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(key)
	cmds = append(cmds, cmd)
	return m, tea.Batch(cmds...)
}

func operationCmd(label string, fn func() error) tea.Cmd {
	return func() tea.Msg { return operationMsg{label: label, err: fn()} }
}

func (m *Model) openInput(next mode, prompt, placeholder, value string) tea.Cmd {
	m.mode = next
	m.input.Prompt = prompt + ": "
	m.input.Placeholder = placeholder
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.message = "Enter to confirm • Esc to cancel"
	return m.input.Focus()
}

func (m *Model) requireRepo() bool {
	if m.repo != nil {
		return true
	}
	m.message = "Run github-tui-go from inside a Git repository"
	return false
}

func (m *Model) setFiles(files []gitops.FileStatus) {
	items := make([]list.Item, 0, len(files))
	for _, file := range files {
		items = append(items, fileItem{file: file})
	}
	m.status.SetItems(items)
	if len(items) == 0 && m.repo != nil {
		m.status.Title = "Working tree — clean"
	} else {
		m.status.Title = fmt.Sprintf("Working tree — %d changed", len(items))
	}
}

func (m *Model) setBranches(branches []string) {
	items := make([]list.Item, 0, len(branches))
	for _, branch := range branches {
		items = append(items, branchItem(branch))
	}
	m.branchList.SetItems(items)
}

func (m *Model) resize() {
	bodyHeight := max(8, m.height-7)
	leftWidth := max(30, min(54, m.width/2))
	rightWidth := max(24, m.width-leftWidth-5)
	m.status.SetSize(leftWidth-4, bodyHeight-2)
	m.branchList.SetSize(leftWidth-4, bodyHeight-2)
	m.log.Width, m.log.Height = rightWidth-4, bodyHeight-2
	m.input.Width = max(20, m.width-24)
	m.updateLog()
}

func (m *Model) updateLog() {
	var lines []string
	if m.repo != nil {
		lines = append(lines, titleStyle.Render("Repository"), dimStyle.Render(m.repo.Root))
	} else if m.repoErr != nil {
		if errors.Is(m.repoErr, git.ErrRepositoryNotExists) {
			lines = append(lines, errorStyle.Render("Not inside a Git repository"), "", "Change into a Git project and run the app again.")
		} else {
			lines = append(lines, errorStyle.Render("Repository error"), m.repoErr.Error())
		}
	}
	if !m.account.Authenticated {
		lines = append(lines, "")
		lines = append(lines, errorStyle.Render("GitHub CLI is not connected"), "Press g to run gh auth login.")
		if m.authErr != nil {
			lines = append(lines, dimStyle.Render(m.authErr.Error()))
		}
	}
	if m.showHelp {
		help, err := glamour.Render("## Daily workflow\n\n- `b` create a branch from master\n- `u` update working branch with origin/master\n- `c` commit all changes\n- `p` push current branch\n- `P` open GitHub pull request form\n- `s` stash changes\n- `S` pop latest stash\n\n## Other\n\n- `o` checkout local branch\n- `x` discard selected file\n- `X` discard all changes\n- `g` connect GitHub CLI\n- `r` refresh\n- `q` quit\n", "dark")
		if err == nil {
			lines = append(lines, "", help)
		}
	} else {
		lines = append(lines, "", dimStyle.Render("Press ? for full help"))
	}
	m.log.SetContent(strings.Join(lines, "\n"))
}

func (m Model) View() string {
	if m.width == 0 {
		return "Starting github-tui-go…"
	}
	header := m.navbar()
	bodyHeight := max(8, m.height-7)
	leftWidth := max(30, min(54, m.width/2))
	rightWidth := max(24, m.width-leftWidth-5)

	left := m.status.View()
	if m.mode == modeCheckout {
		left = m.branchList.View()
	}
	left = panelStyle.Width(leftWidth).Height(bodyHeight).Render(left)
	right := panelStyle.Width(rightWidth).Height(bodyHeight).Render(m.log.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	footer := m.message
	if m.busy {
		footer = m.spinner.View() + " " + footer
	}
	if m.mode == modeCommit || m.mode == modeBranchName || m.mode == modeStash {
		footer = m.input.View() + "\n" + dimStyle.Render("Enter confirm • Esc cancel")
	} else if m.mode == modeNormal {
		footer += "\n" + dimStyle.Render("b branch • u update • c commit • p push • P pull request • s/S stash • ? help • q quit")
	}
	return header + "\n" + body + "\n" + footer
}

func (m Model) navbar() string {
	project := "not a repository"
	if m.repo != nil {
		project = filepath.Base(m.repo.Root)
	}
	branch := m.branch
	if branch == "" {
		branch = "—"
	}

	connection := errorStyle.Render("● DISCONNECTED")
	username := dimStyle.Render("press g to connect")
	if m.account.Authenticated {
		connection = onlineStyle.Render("● CONNECTED")
		username = accentStyle.Render("@" + m.account.Login)
	}

	separator := dimStyle.Render(" │ ")
	nav := titleStyle.Render("github-tui-go") +
		separator + dimStyle.Render("PROJECT ") + accentStyle.Render(project) +
		separator + dimStyle.Render("BRANCH ") + accentStyle.Render(branch) +
		separator + connection + " " + username

	return navbarStyle.Width(max(1, m.width-2)).MaxWidth(max(1, m.width)).Render(nav)
}

func statusLabel(file gitops.FileStatus) string {
	return fmt.Sprintf("index %s  worktree %s", codeLabel(file.Staging), codeLabel(file.Worktree))
}

func codeLabel(code git.StatusCode) string {
	switch code {
	case git.Unmodified:
		return "—"
	case git.Untracked:
		return "untracked"
	case git.Modified:
		return "modified"
	case git.Added:
		return "added"
	case git.Deleted:
		return "deleted"
	case git.Renamed:
		return "renamed"
	case git.Copied:
		return "copied"
	case git.UpdatedButUnmerged:
		return "conflict"
	default:
		return string(code)
	}
}
