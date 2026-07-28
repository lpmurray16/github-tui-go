package githubauth

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/google/go-github/v74/github"
)

type Account struct {
	Authenticated bool
	Login         string
	Name          string
}

func Status() (Account, error) {
	if err := exec.Command("gh", "auth", "status").Run(); err != nil {
		return Account{}, fmt.Errorf("GitHub CLI is not authenticated: %w", err)
	}

	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return Account{}, fmt.Errorf("read GitHub CLI token: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return Account{}, fmt.Errorf("GitHub CLI returned an empty token")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := github.NewClient(nil).WithAuthToken(token)
	user, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return Account{}, fmt.Errorf("verify GitHub account: %w", err)
	}
	return Account{Authenticated: true, Login: user.GetLogin(), Name: user.GetName()}, nil
}

func Token() (string, error) {
	out, err := exec.Command("gh", "auth", "token").Output()
	if err != nil {
		return "", fmt.Errorf("read GitHub CLI token: %w", err)
	}
	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("GitHub CLI returned an empty token")
	}
	return token, nil
}

func OpenPRForm(repositoryRoot, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" || strings.Contains(branch, "detached") {
		return fmt.Errorf("a working branch is required to create a pull request")
	}
	if branch == "master" {
		return fmt.Errorf("switch to a working branch before creating a pull request")
	}
	cmd := exec.Command("gh", "pr", "create", "--web", "--base", "master", "--head", branch)
	cmd.Dir = repositoryRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("open pull request form: %s", detail)
	}
	return nil
}

func PublishRepository(repositoryRoot, name string, public, push bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("repository name is required")
	}
	visibility := "--private"
	if public {
		visibility = "--public"
	}
	args := []string{"repo", "create", name, visibility, "--source", repositoryRoot, "--remote", "origin"}
	if push {
		args = append(args, "--push")
	}
	cmd := exec.Command("gh", args...)
	cmd.Dir = repositoryRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return fmt.Errorf("publish repository: %s", detail)
	}
	return nil
}
