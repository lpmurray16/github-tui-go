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
	"github.com/lpmurray16/github-tui-go/internal/appconfig"
	"github.com/lpmurray16/github-tui-go/internal/githubauth"
	"github.com/lpmurray16/github-tui-go/internal/gitops"
)

type mode int

const (
	modeNormal mode = iota
	modeCommit
	modeBranchName
	modeStash
	modePublishName
	modePublishConfirm
	modeLogoutConfirm
	modeWorkspaceRoot
	modeProjectSwitch
	modeCheckout
	modeDiscardFile
	modeDiscardAll
)

var (
	black         = lipgloss.Color("#000000")
	surface       = lipgloss.Color("#070707")
	orange        = lipgloss.Color("#ff6b1a")
	orangeMuted   = lipgloss.Color("#6b3214")
	cyan          = lipgloss.Color("#00ffff")
	cyanMuted     = lipgloss.Color("#14535b")
	green         = lipgloss.Color("#44d17a")
	text          = lipgloss.Color("#d7d7d7")
	dim           = lipgloss.Color("#737373")
	red           = lipgloss.Color("#ff4057")
	baseStyle     = lipgloss.NewStyle().Background(black).Foreground(text)
	panelStyle    = lipgloss.NewStyle().Background(black).Foreground(text).Border(lipgloss.RoundedBorder()).BorderForeground(orangeMuted).Padding(0, 1)
	detailStyle   = lipgloss.NewStyle().Background(black).Foreground(text).Border(lipgloss.RoundedBorder()).BorderForeground(cyanMuted).Padding(0, 1)
	navbarStyle   = lipgloss.NewStyle().Background(surface).Foreground(text).Padding(0, 1).Bold(true)
	footerStyle   = lipgloss.NewStyle().Background(surface).Foreground(text).Padding(0, 1)
	activityStyle = lipgloss.NewStyle().Background(black).Foreground(text).PaddingLeft(1)
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(orange)
	accentStyle   = lipgloss.NewStyle().Bold(true).Foreground(cyan)
	onlineStyle   = lipgloss.NewStyle().Bold(true).Foreground(green)
	errorStyle    = lipgloss.NewStyle().Bold(true).Foreground(red)
	dimStyle      = lipgloss.NewStyle().Foreground(dim)
	labelStyle    = lipgloss.NewStyle().Foreground(dim).Bold(true)
	valueStyle    = lipgloss.NewStyle().Foreground(text)
	keyStyle      = lipgloss.NewStyle().Background(lipgloss.Color("#1b1b1b")).Foreground(cyan).Bold(true).Padding(0, 1)
)

type fileItem struct{ file gitops.FileStatus }

func (i fileItem) Title() string       { return i.file.Path }
func (i fileItem) Description() string { return statusLabel(i.file) }
func (i fileItem) FilterValue() string { return i.file.Path }

type branchItem string

func (i branchItem) Title() string       { return string(i) }
func (i branchItem) Description() string { return "local branch" }
func (i branchItem) FilterValue() string { return string(i) }

type projectItem struct{ project gitops.Project }

func (i projectItem) Title() string {
	marker := "✓"
	if i.project.Dirty {
		marker = "●"
	}
	return marker + "  " + i.project.Name
}
func (i projectItem) Description() string {
	state := "clean"
	if i.project.Dirty {
		state = "changed"
	}
	return fmt.Sprintf("branch %-18s  %s", i.project.Branch, state)
}
func (i projectItem) FilterValue() string { return i.project.Name + " " + i.project.Path }

type startupMsg struct {
	repo         *gitops.Repository
	files        []gitops.FileStatus
	branches     []string
	branch       string
	hasOrigin    bool
	projectsRoot string
	projects     []gitops.Project
	account      githubauth.Account
	repoErr      error
	authErr      error
	configErr    error
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
	configErr     error
	account       githubauth.Account
	branch        string
	branches      []string
	hasOrigin     bool
	projectsRoot  string
	projects      []gitops.Project
	status        list.Model
	branchList    list.Model
	projectList   list.Model
	log           viewport.Model
	input         textinput.Model
	spinner       spinner.Model
	publishName   string
	message       string
	showHelp      bool
}

func New() Model {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(text)
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(dim)
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(orange).Bold(true).BorderLeftForeground(orange)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(cyan).BorderLeftForeground(orange)
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.Foreground(dim)
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.Foreground(lipgloss.Color("#444444"))

	status := list.New(nil, delegate, 46, 18)
	status.Title = "Working tree"
	status.SetShowHelp(false)
	status.SetFilteringEnabled(true)
	status.Styles.Title = titleStyle

	branches := list.New(nil, delegate, 46, 18)
	branches.Title = "Checkout branch"
	branches.SetShowHelp(false)
	branches.Styles.Title = titleStyle

	projects := list.New(nil, delegate, 46, 18)
	projects.Title = "Switch project"
	projects.SetShowHelp(false)
	projects.SetFilteringEnabled(true)
	projects.Styles.Title = titleStyle

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

	return Model{status: status, branchList: branches, projectList: projects, input: input, spinner: spin, log: log, message: "Loading repository and GitHub account…"}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(loadCmd(""), m.spinner.Tick)
}

func loadCmd(start string) tea.Cmd {
	return func() tea.Msg {
		result := startupMsg{}
		if start == "" {
			cwd, err := os.Getwd()
			if err != nil {
				result.repoErr = err
			} else {
				start = cwd
			}
		}
		if result.repoErr == nil {
			result.repo, result.repoErr = gitops.Open(start)
		}
		if result.repoErr == nil {
			result.files, result.repoErr = result.repo.Status()
			if result.repoErr == nil {
				result.branch, result.repoErr = result.repo.CurrentBranch()
			}
			if result.repoErr == nil {
				result.branches, result.repoErr = result.repo.Branches()
			}
			if result.repoErr == nil {
				result.hasOrigin = result.repo.HasOrigin()
			}
		}
		cfg, configErr := appconfig.Load()
		result.configErr = configErr
		result.projectsRoot = cfg.ProjectsRoot
		if configErr == nil && cfg.ProjectsRoot != "" {
			result.projects, result.configErr = gitops.DiscoverProjects(cfg.ProjectsRoot)
		}
		result.account, result.authErr = githubauth.Status()
		return result
	}
}

func (m Model) activePath() string {
	if m.repo != nil {
		return m.repo.Root
	}
	return ""
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
		m.projectsRoot, m.projects, m.configErr = msg.projectsRoot, msg.projects, msg.configErr
		m.branch, m.branches, m.hasOrigin = msg.branch, msg.branches, msg.hasOrigin
		m.setFiles(msg.files)
		m.setBranches(msg.branches)
		m.setProjects(msg.projects)
		if strings.HasPrefix(m.message, "Switching to ") && m.repo != nil {
			m.message = "Switched to " + filepath.Base(m.repo.Root)
		} else if strings.HasPrefix(m.message, "Loading repository") {
			m.message = "Ready"
		}
		m.updateLog()
	case operationMsg:
		m.busy = false
		if msg.err != nil {
			m.message = "Error: " + msg.err.Error()
		} else {
			m.message = msg.label
		}
		m.mode = modeNormal
		return m, loadCmd(m.activePath())
	case authLoginMsg:
		m.busy = false
		if msg.err != nil {
			m.message = "GitHub login failed: " + msg.err.Error()
		} else {
			m.message = "GitHub login completed"
		}
		return m, loadCmd(m.activePath())
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
			cmds = append(cmds, loadCmd(m.activePath()))
		case "tab":
			if m.projectsRoot == "" {
				m.message = "Root project directory missing — press W to configure it"
			} else if len(m.projects) == 0 {
				m.message = "No Git repositories found directly inside the configured root"
			} else {
				m.mode = modeProjectSwitch
			}
		case "W":
			root := m.projectsRoot
			if root == "" && m.repo != nil {
				root = filepath.Dir(m.repo.Root)
			}
			cmds = append(cmds, m.openInput(modeWorkspaceRoot, "Projects root", "Path containing your project folders", root))
		case "0":
			if m.account.Authenticated {
				m.mode = modeLogoutConfirm
				m.message = fmt.Sprintf("Log out of GitHub as @%s? [y/N]", m.account.Login)
			} else {
				m.busy, m.message = true, "Opening GitHub CLI login…"
				cmd := exec.Command("gh", "auth", "login")
				cmds = append(cmds, tea.ExecProcess(cmd, func(err error) tea.Msg { return authLoginMsg{err: err} }))
			}
		case "g", "G":
			if m.requireRepo() {
				if m.hasOrigin {
					m.message = "This repository already has an origin remote"
				} else if !m.account.Authenticated {
					m.message = "Connect GitHub first with 0"
				} else {
					name := filepath.Base(m.repo.Root)
					cmds = append(cmds, m.openInput(modePublishName, "GitHub repository", "owner/repository", name))
				}
			}
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
				m.busy, m.message = true, "Updating from origin/master, then origin/main if needed…"
				cmds = append(cmds, operationCmd("Current branch updated from the remote base branch", m.repo.UpdateFromPrimaryBranch))
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
	} else if m.mode == modeProjectSwitch {
		var cmd tea.Cmd
		m.projectList, cmd = m.projectList.Update(msg)
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
	case modePublishName:
		if key.String() == "enter" {
			m.publishName = strings.TrimSpace(m.input.Value())
			m.input.Blur()
			if m.publishName == "" {
				m.message = "Repository name is required"
				return m, tea.Batch(cmds...)
			}
			m.mode = modePublishConfirm
			m.message = fmt.Sprintf("Publish %s as PUBLIC? [Y] public • [p] private • Esc cancel", m.publishName)
			return m, tea.Batch(cmds...)
		}
	case modePublishConfirm:
		public := true
		switch key.String() {
		case "p":
			public = false
		case "enter", "y", "Y":
			public = true
		default:
			return m, tea.Batch(cmds...)
		}
		visibility := "public"
		if !public {
			visibility = "private"
		}
		push := m.repo.HasCommits()
		m.busy, m.message = true, fmt.Sprintf("Publishing %s as %s…", m.publishName, visibility)
		cmds = append(cmds, operationCmd(fmt.Sprintf("Published %s as %s and configured origin", m.publishName, visibility), func() error {
			return githubauth.PublishRepository(m.repo.Root, m.publishName, public, push)
		}))
		return m, tea.Batch(cmds...)
	case modeLogoutConfirm:
		if strings.EqualFold(key.String(), "y") {
			login := m.account.Login
			m.busy, m.message = true, "Logging out of GitHub…"
			cmds = append(cmds, operationCmd("Logged out of GitHub", func() error { return githubauth.Logout(login) }))
			return m, tea.Batch(cmds...)
		}
		m.mode, m.message = modeNormal, "Logout cancelled"
		return m, tea.Batch(cmds...)
	case modeWorkspaceRoot:
		if key.String() == "enter" {
			root := strings.TrimSpace(m.input.Value())
			m.input.Blur()
			m.busy, m.message = true, "Saving projects root…"
			cmds = append(cmds, operationCmd("Projects root saved — press Tab to switch projects", func() error {
				return appconfig.SaveProjectsRoot(root)
			}))
			return m, tea.Batch(cmds...)
		}
	case modeProjectSwitch:
		if key.String() == "enter" {
			if item, ok := m.projectList.SelectedItem().(projectItem); ok {
				m.mode = modeNormal
				m.busy, m.message = true, "Switching to "+item.project.Name+"…"
				return m, loadCmd(item.project.Path)
			}
		}
		var cmd tea.Cmd
		m.projectList, cmd = m.projectList.Update(key)
		cmds = append(cmds, cmd)
		return m, tea.Batch(cmds...)
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

func (m *Model) setProjects(projects []gitops.Project) {
	items := make([]list.Item, 0, len(projects))
	for _, project := range projects {
		items = append(items, projectItem{project: project})
	}
	m.projectList.SetItems(items)
	m.projectList.Title = fmt.Sprintf("Switch project — %d repositories", len(items))
}

func (m *Model) resize() {
	bodyHeight := max(8, m.height-7)
	leftWidth := max(30, min(54, m.width/2))
	rightWidth := max(24, m.width-leftWidth-5)
	m.status.SetSize(leftWidth-4, bodyHeight-2)
	m.branchList.SetSize(leftWidth-4, bodyHeight-2)
	m.projectList.SetSize(leftWidth-4, bodyHeight-2)
	m.log.Width, m.log.Height = rightWidth-4, bodyHeight-2
	m.input.Width = max(20, m.width-24)
	m.updateLog()
}

func (m *Model) updateLog() {
	var lines []string
	lines = append(lines, sectionTitle("REPOSITORY OVERVIEW"))

	if m.repo != nil {
		origin := onlineStyle.Render("✓ configured")
		if !m.hasOrigin {
			origin = errorStyle.Render("● missing")
		}
		lines = append(lines,
			detailRow("PROJECT", filepath.Base(m.repo.Root)),
			detailRow("BRANCH", m.branch),
			detailRow("ORIGIN", origin),
			"",
			dimStyle.Render(m.repo.Root),
		)
		if !m.hasOrigin {
			lines = append(lines, "", errorStyle.Render("Origin is not configured"), dimStyle.Render("Press g to publish this project to GitHub."))
		}
	} else if m.repoErr != nil {
		if errors.Is(m.repoErr, git.ErrRepositoryNotExists) {
			lines = append(lines, errorStyle.Render("● NOT A GIT REPOSITORY"), "", dimStyle.Render("Switch projects with Tab or launch inside a Git project."))
		} else {
			lines = append(lines, errorStyle.Render("● REPOSITORY ERROR"), dimStyle.Render(m.repoErr.Error()))
		}
	}

	lines = append(lines, "", sectionTitle("WORKSPACE"))
	if m.projectsRoot == "" {
		lines = append(lines, errorStyle.Render("● ROOT PROJECT DIRECTORY MISSING"), dimStyle.Render("Press W to assign the folder containing your projects."))
	} else {
		lines = append(lines,
			detailRow("ROOT", filepath.Base(m.projectsRoot)),
			detailRow("PROJECTS", fmt.Sprintf("%d repositories", len(m.projects))),
			"",
			dimStyle.Render(m.projectsRoot),
			dimStyle.Render("Press Tab to search and switch projects."),
		)
	}
	if m.configErr != nil {
		lines = append(lines, errorStyle.Render("Workspace error: "+m.configErr.Error()))
	}

	lines = append(lines, "", sectionTitle("SESSION"))
	if m.account.Authenticated {
		lines = append(lines, detailRow("GITHUB", onlineStyle.Render("● @"+m.account.Login)))
	} else {
		lines = append(lines, detailRow("GITHUB", errorStyle.Render("● disconnected")), dimStyle.Render("Press 0 to connect with GitHub CLI."))
		if m.authErr != nil {
			lines = append(lines, dimStyle.Render(m.authErr.Error()))
		}
	}
	if m.message != "" {
		lines = append(lines, detailRow("ACTIVITY", m.message))
	}

	if m.showHelp {
		help, err := glamour.Render("## Workspace\n\n- `Tab` switch projects\n- `W` set or change projects root\n\n## Daily workflow\n\n- `g` / `G` publish project when origin is missing\n- `b` create a branch from master\n- `u` update from origin/master, then origin/main\n- `c` commit all changes\n- `p` push current branch\n- `P` open GitHub pull request form\n- `s` stash changes\n- `S` pop latest stash\n\n## Other\n\n- `0` log in or log out of GitHub\n- `o` checkout local branch\n- `x` discard selected file\n- `X` discard all changes\n- `r` refresh\n- `q` quit\n", "dark")
		if err == nil {
			lines = append(lines, "", help)
		}
	} else {
		lines = append(lines, "", dimStyle.Render("Press ? for full help"))
	}
	m.log.SetContent(strings.Join(lines, "\n"))
}

func sectionTitle(label string) string {
	return titleStyle.Render("◆ "+label) + " " + dimStyle.Render(strings.Repeat("─", 18))
}

func detailRow(label, value string) string {
	return labelStyle.Width(11).Render(label) + valueStyle.Render(value)
}

func (m Model) View() string {
	if m.width == 0 {
		return baseStyle.Render("Starting github-tui-go…")
	}
	header := m.navbar()
	bodyHeight := max(8, m.height-7)
	leftWidth := max(30, min(54, m.width/2))
	rightWidth := max(24, m.width-leftWidth-5)

	left := m.status.View()
	if m.mode == modeCheckout {
		left = m.branchList.View()
	} else if m.mode == modeProjectSwitch {
		left = m.projectList.View()
	} else if len(m.status.Items()) == 0 {
		left = m.cleanState(leftWidth-4, bodyHeight-2)
	}
	left = panelStyle.Width(leftWidth).Height(bodyHeight).Render(left)
	right := detailStyle.Width(rightWidth).Height(bodyHeight).Render(m.log.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)

	activity := m.message
	if activity == "" {
		activity = "Ready"
	}
	if m.busy {
		activity = m.spinner.View() + " " + activity
	} else {
		activity = accentStyle.Render("›") + " " + activity
	}
	footer := activityStyle.Width(max(1, m.width-2)).Render(activity)

	switch {
	case m.mode == modeCommit || m.mode == modeBranchName || m.mode == modeStash || m.mode == modePublishName || m.mode == modeWorkspaceRoot:
		footer += "\n" + footerStyle.Width(max(1, m.width-2)).Render(m.input.View()+"  "+shortcut("Enter", "Confirm")+"  "+shortcut("Esc", "Cancel"))
	case m.mode == modeProjectSwitch:
		footer += "\n" + footerStyle.Width(max(1, m.width-2)).Render(shortcut("↑/↓", "Navigate")+"  "+shortcut("Enter", "Switch")+"  "+shortcut("/", "Filter")+"  "+shortcut("Esc", "Cancel"))
	case m.mode == modeCheckout:
		footer += "\n" + footerStyle.Width(max(1, m.width-2)).Render(shortcut("↑/↓", "Navigate")+"  "+shortcut("Enter", "Checkout")+"  "+shortcut("Esc", "Cancel"))
	case m.mode == modeNormal:
		shortcuts := []string{
			shortcut("Tab", "Projects"),
			shortcut("W", "Root"),
			shortcut("b", "Branch"),
			shortcut("u", "Update"),
			shortcut("c", "Commit"),
			shortcut("p", "Push"),
			shortcut("P", "PR"),
			shortcut("?", "Help"),
		}
		if !m.hasOrigin {
			shortcuts = append([]string{shortcut("g", "Publish")}, shortcuts...)
		}
		footer += "\n" + footerStyle.Width(max(1, m.width-2)).Render(strings.Join(shortcuts, "  "))
	}

	screen := header + "\n" + body + "\n" + footer
	return baseStyle.Width(max(1, m.width)).Height(max(1, m.height)).Render(screen)
}

func (m Model) cleanState(width, height int) string {
	title := sectionTitle("WORKING TREE")
	if m.repo == nil {
		content := errorStyle.Render("● NO REPOSITORY") + "\n" + dimStyle.Render("Press Tab to select a project")
		return title + "\n" + lipgloss.NewStyle().Width(width).PaddingTop(max(1, height/3)).Align(lipgloss.Center).Render(content)
	}
	content := onlineStyle.Render("✓  CLEAN") + "\n\n" + valueStyle.Render("Nothing to commit") + "\n" + dimStyle.Render("Your working tree is up to date")
	return title + "\n" + lipgloss.NewStyle().Width(width).PaddingTop(max(1, height/3-2)).Align(lipgloss.Center).Render(content)
}

func shortcut(key, label string) string {
	return keyStyle.Render(key) + " " + dimStyle.Render(label)
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

	root := errorStyle.Render("● ROOT MISSING")
	if m.projectsRoot != "" {
		root = labelStyle.Render("ROOT ") + accentStyle.Render(filepath.Base(m.projectsRoot))
	}
	origin := errorStyle.Render("● NO ORIGIN")
	if m.hasOrigin {
		origin = onlineStyle.Render("✓ ORIGIN")
	}
	account := errorStyle.Render("● DISCONNECTED")
	if m.account.Authenticated {
		account = onlineStyle.Render("● CONNECTED") + " " + accentStyle.Render("@"+m.account.Login)
	}

	brand := lipgloss.NewStyle().Background(orange).Foreground(black).Bold(true).Padding(0, 1).Render("GITHUB TUI")
	separator := dimStyle.Render("  │  ")
	nav := brand + "  " + root +
		separator + labelStyle.Render("PROJECT ") + titleStyle.Render(project) +
		separator + labelStyle.Render("BRANCH ") + accentStyle.Render(branch) +
		separator + origin +
		separator + account

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
