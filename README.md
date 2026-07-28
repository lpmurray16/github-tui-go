# github-tui-go

A keyboard-driven Git and GitHub terminal interface built with Go, Bubble Tea, Lip Gloss, Bubbles, go-git, go-github, and Glamour.

## Features

- Reuses your existing GitHub CLI login (`gh auth login`)
- Detects whether the current directory (or a parent) is a Git repository
- Saves a configurable projects-root directory and switches between sibling Git repositories without leaving the TUI
- Detects a missing `origin` and publishes the local project to a public or private GitHub repository
- Shows staged and unstaged status
- Commits all current changes
- Pushes the current branch to `origin`
- Fetches the latest `master`, then creates and checks out a working branch from it
- Updates the working branch from `origin/master`, falling back to `origin/main`
- Automatically refreshes only the affected repository, workspace, or account state after actions, avoiding unnecessary sibling-project scans and GitHub API calls
- Opens GitHub's pull request form in your browser for final review
- Stashes tracked/untracked changes and pops the latest stash
- Checks out existing local branches
- Discards one selected file or all tracked and untracked changes, with confirmation

## Requirements

- Go 1.26.5 or newer
- GitHub CLI (`gh`)
- An HTTPS `origin` remote for token-authenticated pushes

## Run

Run the application from anywhere inside a Git repository:

```sh
go run .
```

Or build it:

```sh
go build -o github-tui-go.exe .
./github-tui-go.exe
```

If GitHub CLI is not authenticated, press `0` in the TUI to launch `gh auth login`.

## Projects workspace

Press `W` to enter the directory that contains your project folders. The path is validated and saved in your user config directory under `github-tui-go/config.json`.

Each direct child folder containing a `.git` repository appears in the project switcher. Press `Tab`, type to filter, use the arrow keys, and press `Enter` to switch the active repository. All subsequent Git and GitHub actions target that selected project.

## Keys

| Key | Action |
| --- | --- |
| `r` | Quickly refresh the active repository |
| `R` | Fully rescan the repository, workspace projects, and GitHub account |
| `Tab` | Open the searchable project switcher |
| `W` | Set or change the projects-root directory |
| `g` / `G` | Publish the project to GitHub and configure `origin` |
| `c` | Commit all changes |
| `p` | Push current branch |
| `P` | Open GitHub's pull request form for the working branch |
| `b` | Create a branch from the latest `master` |
| `u` | Update from `origin/master`, falling back to `origin/main` |
| `s` | Stash tracked and untracked changes |
| `S` | Pop the latest stash |
| `o` | Open branch checkout picker |
| `x` | Discard the selected file |
| `X` | Discard all changes |
| `0` | Log in or log out of GitHub |
| `?` | Toggle full help |
| `q` | Quit |

> **Warning:** discard operations are destructive. The app asks for confirmation before executing them.
