package preflight

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

type Manager string

const (
	ManagerNone       Manager = ""
	ManagerWinget     Manager = "winget"
	ManagerChocolatey Manager = "chocolatey"
)

type Tool struct {
	Name         string
	Command      string
	Path         string
	WingetID     string
	ChocoPackage string
	DownloadURL  string
}

type Result struct {
	Tools   []Tool
	Manager Manager
}

func Check() Result {
	tools := []Tool{
		{Name: "Git for Windows", Command: "git", WingetID: "Git.Git", ChocoPackage: "git", DownloadURL: "https://git-scm.com/download/win"},
		{Name: "GitHub CLI", Command: "gh", WingetID: "GitHub.cli", ChocoPackage: "gh", DownloadURL: "https://cli.github.com/"},
	}
	for index := range tools {
		tools[index].Path, _ = exec.LookPath(tools[index].Command)
	}
	manager := ManagerNone
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("winget"); err == nil {
			manager = ManagerWinget
		} else if _, err := exec.LookPath("choco"); err == nil {
			manager = ManagerChocolatey
		}
	}
	return Result{Tools: tools, Manager: manager}
}

func (r Result) Missing() []Tool {
	missing := make([]Tool, 0)
	for _, tool := range r.Tools {
		if tool.Path == "" {
			missing = append(missing, tool)
		}
	}
	return missing
}

func (r Result) AllAvailable() bool { return len(r.Missing()) == 0 }

func (r Result) InstallCommand() (*exec.Cmd, string, error) {
	missing := r.Missing()
	if len(missing) == 0 {
		return nil, "", fmt.Errorf("all required tools are already installed")
	}
	switch r.Manager {
	case ManagerWinget:
		commands := make([]string, 0, len(missing))
		for _, tool := range missing {
			commands = append(commands, fmt.Sprintf("winget install --id %s --exact --accept-source-agreements --accept-package-agreements", tool.WingetID))
		}
		line := strings.Join(commands, " && ")
		return exec.Command("cmd.exe", "/D", "/S", "/C", line), line, nil
	case ManagerChocolatey:
		packages := make([]string, 0, len(missing))
		for _, tool := range missing {
			packages = append(packages, tool.ChocoPackage)
		}
		args := append([]string{"install"}, packages...)
		args = append(args, "-y")
		line := "choco " + strings.Join(args, " ")
		return exec.Command("choco", args...), line, nil
	default:
		return nil, "", fmt.Errorf("winget and Chocolatey were not found")
	}
}
