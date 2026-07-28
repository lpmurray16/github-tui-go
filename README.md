# github-tui-go

A keyboard-driven Git and GitHub terminal interface built with Go, Bubble Tea, Lip Gloss, Bubbles, go-git, go-github, and Glamour.

## Features

- Reuses your existing GitHub CLI login (`gh auth login`)
- Detects whether the current directory (or a parent) is a Git repository
- Detects a missing `origin` and publishes the local project to a public or private GitHub repository
- Shows staged and unstaged status
- Commits all current changes
- Pushes the current branch to `origin`
- Fetches the latest `master`, then creates and checks out a working branch from it
- Updates the working branch by fetching and merging `origin/master`
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

If GitHub CLI is not authenticated, press `g` in the TUI to launch `gh auth login`.

## Keys

| Key | Action |
| --- | --- |
| `r` | Refresh repository and account status |
| `g` / `G` | Publish the project to GitHub and configure `origin` |
| `c` | Commit all changes |
| `p` | Push current branch |
| `P` | Open GitHub's pull request form for the working branch |
| `b` | Create a branch from the latest `master` |
| `u` | Fetch and merge `origin/master` into the working branch |
| `s` | Stash tracked and untracked changes |
| `S` | Pop the latest stash |
| `o` | Open branch checkout picker |
| `x` | Discard the selected file |
| `X` | Discard all changes |
| `0` | Log in or log out of GitHub |
| `?` | Toggle full help |
| `q` | Quit |

> **Warning:** discard operations are destructive. The app asks for confirmation before executing them.
