package gitops

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/lpmurray16/github-tui-go/internal/githubauth"
)

type Repository struct {
	Repo *git.Repository
	Root string
}

type FileStatus struct {
	Path     string
	Staging  git.StatusCode
	Worktree git.StatusCode
}

type Project struct {
	Name   string
	Path   string
	Branch string
	Dirty  bool
}

const maxDiffPreviewBytes = 200_000

func (r *Repository) FileDiff(file FileStatus) (string, error) {
	path := filepath.ToSlash(filepath.Clean(file.Path))
	if path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid repository path %q", path)
	}
	if file.Worktree == git.Untracked {
		untracked, err := os.Open(filepath.Join(r.Root, filepath.FromSlash(path)))
		if err != nil {
			return "", fmt.Errorf("open untracked file: %w", err)
		}
		defer untracked.Close()
		contents, err := io.ReadAll(io.LimitReader(untracked, maxDiffPreviewBytes+1))
		if err != nil {
			return "", fmt.Errorf("read untracked file: %w", err)
		}
		if bytes.IndexByte(contents, 0) >= 0 {
			return "Binary file — preview unavailable", nil
		}
		lines := strings.Split(string(contents), "\n")
		for index := range lines {
			lines[index] = "+" + lines[index]
		}
		return truncateDiff(fmt.Sprintf("new file: %s\n--- /dev/null\n+++ b/%s\n@@ new file @@\n%s", path, path, strings.Join(lines, "\n"))), nil
	}

	sections := make([]string, 0, 2)
	if file.Staging != git.Unmodified {
		output, err := r.gitOutput("diff", "--cached", "--no-ext-diff", "--no-color", "--", path)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(output) != "" {
			sections = append(sections, "STAGED CHANGES\n"+output)
		}
	}
	if file.Worktree != git.Unmodified {
		output, err := r.gitOutput("diff", "--no-ext-diff", "--no-color", "--", path)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(output) != "" {
			sections = append(sections, "WORKTREE CHANGES\n"+output)
		}
	}
	if len(sections) == 0 {
		return "No textual diff available for this file.", nil
	}
	return truncateDiff(strings.Join(sections, "\n\n")), nil
}

func truncateDiff(diff string) string {
	if len(diff) <= maxDiffPreviewBytes {
		return diff
	}
	return diff[:maxDiffPreviewBytes] + "\n\n… diff preview truncated …"
}

func DiscoverProjects(root string) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("scan projects root: %w", err)
	}
	projects := make([]Project, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
			continue
		}
		repo, err := Open(path)
		if err != nil {
			continue
		}
		branch, branchErr := repo.CurrentBranch()
		if branchErr != nil {
			branch = "unknown"
		}
		status, statusErr := repo.Status()
		projects = append(projects, Project{
			Name:   entry.Name(),
			Path:   path,
			Branch: branch,
			Dirty:  statusErr == nil && len(status) > 0,
		})
	}
	sort.Slice(projects, func(i, j int) bool { return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name) })
	return projects, nil
}

func Open(start string) (*Repository, error) {
	root, err := findRoot(start)
	if err != nil {
		return nil, err
	}
	repo, err := git.PlainOpen(root)
	if err != nil {
		return nil, err
	}
	return &Repository{Repo: repo, Root: root}, nil
}

func findRoot(start string) (string, error) {
	current, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(current); statErr == nil && !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", git.ErrRepositoryNotExists
		}
		current = parent
	}
}

func (r *Repository) Status() ([]FileStatus, error) {
	wt, err := r.Repo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := wt.Status()
	if err != nil {
		return nil, err
	}
	files := make([]FileStatus, 0, len(status))
	for path, state := range status {
		files = append(files, FileStatus{Path: path, Staging: state.Staging, Worktree: state.Worktree})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func (r *Repository) CurrentBranch() (string, error) {
	head, err := r.Repo.Head()
	if err != nil {
		if errors.Is(err, plumbing.ErrReferenceNotFound) {
			ref, refErr := r.Repo.Reference(plumbing.HEAD, false)
			if refErr == nil && ref.Type() == plumbing.SymbolicReference {
				return ref.Target().Short(), nil
			}
		}
		return "", err
	}
	if !head.Name().IsBranch() {
		return head.Hash().String()[:8] + " (detached)", nil
	}
	return head.Name().Short(), nil
}

func (r *Repository) Branches() ([]string, error) {
	iter, err := r.Repo.Branches()
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var branches []string
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		branches = append(branches, ref.Name().Short())
		return nil
	})
	sort.Strings(branches)
	return branches, err
}

func (r *Repository) HasOrigin() bool {
	_, err := r.Repo.Remote("origin")
	return err == nil
}

func (r *Repository) HasCommits() bool {
	_, err := r.Repo.Head()
	return err == nil
}

func (r *Repository) Checkout(branch string) error {
	wt, err := r.Repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName(branch)})
}

func (r *Repository) CreateBranchFrom(name, base string) error {
	name = strings.TrimSpace(name)
	base = strings.TrimSpace(base)
	if name == "" || base == "" {
		return fmt.Errorf("branch name and base branch are required")
	}
	if plumbing.NewBranchReferenceName(name).Validate() != nil {
		return fmt.Errorf("invalid branch name %q", name)
	}
	if _, err := r.Repo.Reference(plumbing.NewBranchReferenceName(name), true); err == nil {
		return fmt.Errorf("branch %q already exists", name)
	}
	hash, err := r.Repo.ResolveRevision(plumbing.Revision(base))
	if err != nil {
		hash, err = r.Repo.ResolveRevision(plumbing.Revision("refs/heads/" + base))
	}
	if err != nil {
		return fmt.Errorf("resolve base branch %q: %w", base, err)
	}
	wt, err := r.Repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(name),
		Hash:   *hash,
		Create: true,
	})
}

func (r *Repository) CreateBranchFromMaster(name string) error {
	if err := r.runGit("fetch", "origin", "master"); err != nil {
		return fmt.Errorf("fetch master: %w", err)
	}
	return r.CreateBranchFrom(name, "refs/remotes/origin/master")
}

func (r *Repository) CommitAll(message, fallbackLogin string) (plumbing.Hash, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return plumbing.ZeroHash, fmt.Errorf("commit message is required")
	}
	wt, err := r.Repo.Worktree()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
		return plumbing.ZeroHash, err
	}
	cfg, err := r.Repo.Config()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	name, email := strings.TrimSpace(cfg.User.Name), strings.TrimSpace(cfg.User.Email)
	if name == "" {
		name = fallbackLogin
	}
	if email == "" && fallbackLogin != "" {
		email = fallbackLogin + "@users.noreply.github.com"
	}
	if name == "" || email == "" {
		return plumbing.ZeroHash, fmt.Errorf("Git author is not configured; set user.name and user.email")
	}
	return wt.Commit(message, &git.CommitOptions{Author: &object.Signature{Name: name, Email: email, When: time.Now()}})
}

func (r *Repository) Push() error {
	branch, err := r.CurrentBranch()
	if err != nil {
		return err
	}
	if strings.Contains(branch, "detached") {
		return fmt.Errorf("cannot push while HEAD is detached")
	}
	remote, err := r.Repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("origin remote: %w", err)
	}
	urls := remote.Config().URLs
	if len(urls) == 0 || !strings.HasPrefix(strings.ToLower(urls[0]), "http") {
		return fmt.Errorf("origin must use an HTTPS URL for GitHub CLI token authentication")
	}
	token, err := githubauth.Token()
	if err != nil {
		return err
	}
	err = r.Repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       &githttp.BasicAuth{Username: "x-access-token", Password: token},
		RefSpecs:   []config.RefSpec{config.RefSpec("refs/heads/" + branch + ":refs/heads/" + branch)},
	})
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

func (r *Repository) UpdateFromPrimaryBranch() error {
	branch, err := r.CurrentBranch()
	if err != nil {
		return err
	}
	if strings.Contains(branch, "detached") {
		return fmt.Errorf("cannot update a detached HEAD")
	}

	masterErr := r.runGit("fetch", "origin", "master")
	if masterErr == nil {
		if err := r.runGit("merge", "origin/master", "--no-edit"); err != nil {
			return fmt.Errorf("fetched master, but merge origin/master failed: %w", err)
		}
		return nil
	}

	mainErr := r.runGit("fetch", "origin", "main")
	if mainErr == nil {
		if err := r.runGit("merge", "origin/main", "--no-edit"); err != nil {
			return fmt.Errorf("master was unavailable; fetched main, but merge origin/main failed: %w", err)
		}
		return nil
	}

	return fmt.Errorf("could not update from either remote base branch; master failed: %v; main failed: %v", masterErr, mainErr)
}

func (r *Repository) Stash(message string) error {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "github-tui-go stash"
	}
	return r.runGit("stash", "push", "-u", "-m", message)
}

func (r *Repository) PopStash() error {
	return r.runGit("stash", "pop")
}

func (r *Repository) gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Root
	output, err := cmd.CombinedOutput()
	if err == nil {
		return string(output), nil
	}
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		detail = strings.Join(args, " ")
	}
	return "", fmt.Errorf("%s: %w", detail, err)
}

func (r *Repository) runGit(args ...string) error {
	_, err := r.gitOutput(args...)
	return err
}

func (r *Repository) DiscardAll() error {
	wt, err := r.Repo.Worktree()
	if err != nil {
		return err
	}
	head, err := r.Repo.Head()
	if err != nil {
		return fmt.Errorf("discard all requires at least one commit: %w", err)
	}
	if err := wt.Reset(&git.ResetOptions{Commit: head.Hash(), Mode: git.HardReset}); err != nil {
		return err
	}
	return wt.Clean(&git.CleanOptions{Dir: true})
}

func (r *Repository) DiscardFile(path string) error {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." || strings.HasPrefix(path, "../") || filepath.IsAbs(path) {
		return fmt.Errorf("invalid repository path %q", path)
	}
	wt, err := r.Repo.Worktree()
	if err != nil {
		return err
	}
	status, err := wt.Status()
	if err != nil {
		return err
	}
	state, ok := status[path]
	if !ok {
		return fmt.Errorf("%s has no changes", path)
	}
	absolute := filepath.Join(r.Root, filepath.FromSlash(path))
	if state.Staging == git.Untracked && state.Worktree == git.Untracked {
		return os.RemoveAll(absolute)
	}

	head, err := r.Repo.Head()
	if err != nil {
		return fmt.Errorf("discard file requires at least one commit: %w", err)
	}
	commit, err := r.Repo.CommitObject(head.Hash())
	if err != nil {
		return err
	}
	tree, err := commit.Tree()
	if err != nil {
		return err
	}
	tracked, fileErr := tree.File(path)
	idx, err := r.Repo.Storer.Index()
	if err != nil {
		return err
	}
	idx.Entries = removeIndexEntry(idx.Entries, path)
	if fileErr != nil {
		if !errors.Is(fileErr, object.ErrFileNotFound) {
			return fileErr
		}
		if err := r.Repo.Storer.SetIndex(idx); err != nil {
			return err
		}
		return os.RemoveAll(absolute)
	}

	reader, err := tracked.Reader()
	if err != nil {
		return err
	}
	contents, err := io.ReadAll(reader)
	reader.Close()
	if err != nil {
		return err
	}
	idx.Entries = append(idx.Entries, &index.Entry{Name: path, Hash: tracked.Hash, Mode: tracked.Mode, Size: uint32(len(contents))})
	sort.Slice(idx.Entries, func(i, j int) bool { return idx.Entries[i].Name < idx.Entries[j].Name })
	if err := r.Repo.Storer.SetIndex(idx); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absolute, contents, 0o644)
}

func removeIndexEntry(entries []*index.Entry, path string) []*index.Entry {
	result := entries[:0]
	for _, entry := range entries {
		if entry.Name != path {
			result = append(result, entry)
		}
	}
	return result
}
