package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"dagger.io/dagger"
	"github.com/mitchellh/go-homedir"
)

func getRepoPath(repoName string) (string, error) {
	return homedir.Expand(fmt.Sprintf(
		"~/.config/container-use/repos/%s",
		filepath.Base(repoName),
	))
}

func (c *Container) BranchName() string {
	return fmt.Sprintf("%s-%s", c.Name, c.ID[:8])
}

func (c *Container) GetWorktreePath() (string, error) {
	return homedir.Expand(fmt.Sprintf("~/.config/container-use/worktrees/%s", c.BranchName()))
}

func (c *Container) InitializeWorktree(localRepoPath string) (string, error) {
	localRepoPath, err := filepath.Abs(localRepoPath)
	if err != nil {
		return "", err
	}

	cuRepoPath, err := InitializeLocalRemote(localRepoPath)
	if err != nil {
		return "", err
	}

	worktreePath, err := c.GetWorktreePath()
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(worktreePath); err == nil {
		return worktreePath, nil
	}

	slog.Info("Initializing worktree", "container-id", c.ID, "container-name", c.Name, "branchName", c.BranchName())
	_, err = runGitCommand(localRepoPath, "fetch", "container-use")
	if err != nil {
		return "", err
	}

	// TODO(braa): safely handle uncommitted changes
	currentBranch, err := runGitCommand(localRepoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	currentBranch = strings.TrimSpace(currentBranch)

	_, err = runGitCommand(localRepoPath, "push", "container-use", currentBranch)
	if err != nil {
		return "", err
	}

	// create worktree, accomodating partial failures where the branch pushed but the worktree wasn't created
	_, err = runGitCommand(cuRepoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", c.BranchName()))
	if err != nil {
		_, err = runGitCommand(cuRepoPath, "worktree", "add", "-b", c.BranchName(), worktreePath, currentBranch)
		if err != nil {
			return "", err
		}
	} else {
		_, err = runGitCommand(cuRepoPath, "worktree", "add", worktreePath, c.BranchName())
		if err != nil {
			return "", err
		}
	}

	_, err = runGitCommand(localRepoPath, "fetch", "container-use")
	if err != nil {
		return "", err
	}

	// set up remote tracking branch if it's not already there
	_, err = runGitCommand(localRepoPath, "show-ref", "--verify", "--quiet", fmt.Sprintf("refs/heads/%s", c.BranchName()))
	if err != nil {
		_, err = runGitCommand(localRepoPath, "branch", "--track", c.BranchName(), fmt.Sprintf("container-use/%s", c.BranchName()))
		if err != nil {
			return "", err
		}
	}

	return worktreePath, nil
}

func InitializeLocalRemote(localRepoPath string) (string, error) {
	localRepoPath, err := filepath.Abs(localRepoPath)
	if err != nil {
		return "", err
	}

	repoName := filepath.Base(localRepoPath)
	cuRepoPath, err := getRepoPath(repoName)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(cuRepoPath); err == nil {
		return cuRepoPath, nil
	}

	slog.Info("Initializing local remote", "local-repo-path", localRepoPath, "container-use-repo-path", cuRepoPath)
	_, err = runGitCommand(localRepoPath, "clone", "--bare", localRepoPath, cuRepoPath)
	if err != nil {
		return "", err
	}

	// set up local remote, updating it if it had been created previously at a different path
	existingURL, err := runGitCommand(localRepoPath, "remote", "get-url", "container-use")
	if err != nil {
		_, err = runGitCommand(localRepoPath, "remote", "add", "container-use", cuRepoPath)
		if err != nil {
			return "", err
		}
	} else {
		existingURL = strings.TrimSpace(existingURL)
		if existingURL != cuRepoPath {
			_, err = runGitCommand(localRepoPath, "remote", "set-url", "container-use", cuRepoPath)
			if err != nil {
				return "", err
			}
		}
	}
	return cuRepoPath, nil
}

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir

	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", fmt.Errorf("git command failed (exit code %d): %w\nOutput: %s",
				exitErr.ExitCode(), err, string(output))
		}
		return "", fmt.Errorf("git command failed: %w", err)
	}

	return string(output), nil
}

func (s *Container) propagateToWorktree(ctx context.Context, name, explanation string) error {
	worktreePath, err := s.GetWorktreePath()
	if err != nil {
		return err
	}

	_, err = s.state.Directory(s.Workdir).Export(
		ctx,
		worktreePath,
		dagger.DirectoryExportOpts{Wipe: true},
	)
	if err != nil {
		return err
	}

	if err := s.commitWorktreeChanges(worktreePath, name, explanation); err != nil {
		return fmt.Errorf("failed to commit worktree changes: %w", err)
	}

	// TODO(braa): "." is a hack here, means we can only work on the repo where we started the server
	localRepoPath, err := filepath.Abs(".")
	if err != nil {
		return err
	}

	_, err = runGitCommand(localRepoPath, "fetch", "container-use")
	return err
}

func (s *Container) commitWorktreeChanges(worktreePath, name, explanation string) error {
	status, err := runGitCommand(worktreePath, "status", "--porcelain")
	if err != nil {
		return err
	}

	if strings.TrimSpace(status) == "" {
		return nil
	}

	if err := s.addNonBinaryFiles(worktreePath); err != nil {
		return err
	}

	commitMsg := fmt.Sprintf("%s\n\n%s", name, explanation)
	_, err = runGitCommand(worktreePath, "commit", "-m", commitMsg)
	return err
}

// AI slop below!
// this is just to keep us moving fast because big git repos get hard to work with
// and our demos like to download large dependencies.
func (s *Container) addNonBinaryFiles(worktreePath string) error {
	statusOutput, err := runGitCommand(worktreePath, "status", "--porcelain")
	if err != nil {
		return err
	}

	lines := strings.Split(strings.TrimSpace(statusOutput), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		if len(line) < 3 {
			continue
		}

		fileName := strings.TrimSpace(line[2:])
		if fileName == "" {
			continue
		}

		if s.shouldSkipFile(fileName) {
			continue
		}

		if !s.isBinaryFile(worktreePath, fileName) {
			_, err = runGitCommand(worktreePath, "add", fileName)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Container) shouldSkipFile(fileName string) bool {
	skipExtensions := []string{
		".tar", ".tar.gz", ".tgz", ".tar.bz2", ".tbz2", ".tar.xz", ".txz",
		".zip", ".rar", ".7z", ".gz", ".bz2", ".xz",
		".exe", ".bin", ".dmg", ".pkg", ".msi",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".tiff", ".svg",
		".mp3", ".mp4", ".avi", ".mov", ".wmv", ".flv", ".mkv",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".so", ".dylib", ".dll", ".a", ".lib",
	}

	lowerName := strings.ToLower(fileName)
	for _, ext := range skipExtensions {
		if strings.HasSuffix(lowerName, ext) {
			return true
		}
	}

	skipPatterns := []string{
		"node_modules/", ".git/", "__pycache__/", ".DS_Store",
		"*.tmp", "*.temp", "*.cache", "*.log",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(lowerName, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

func (s *Container) isBinaryFile(worktreePath, fileName string) bool {
	fullPath := filepath.Join(worktreePath, fileName)

	file, err := os.Open(fullPath)
	if err != nil {
		return true
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return true
	}

	if stat.Size() > 10*1024*1024 {
		return true
	}

	buffer := make([]byte, 8000)
	n, err := file.Read(buffer)
	if err != nil && n == 0 {
		return true
	}

	buffer = buffer[:n]
	if slices.Contains(buffer, 0) {
		return true
	}

	return false
}
