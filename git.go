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

// 10MB
const maxFileSizeForTextCheck = 10 * 1024 * 1024

func getRepoPath(repoName string) (string, error) {
	return homedir.Expand(fmt.Sprintf(
		"~/.config/container-use/repos/%s",
		filepath.Base(repoName),
	))
}

func (c *Environment) BranchName() string {
	return fmt.Sprintf("%s-%s", c.Name, c.ID[:8])
}

func (c *Environment) GetWorktreePath() (string, error) {
	return homedir.Expand(fmt.Sprintf("~/.config/container-use/worktrees/%s", c.BranchName()))
}

func (c *Environment) InitializeWorktree(localRepoPath string) (string, error) {
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

	currentBranch, err := runGitCommand(localRepoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	currentBranch = strings.TrimSpace(currentBranch)

	// this is racy, i think? like if a human is rewriting history on a branch and creating containers, things get complicated.
	// there's only 1 copy of the source branch in the localremote, so there's potential for conflicts.
	_, err = runGitCommand(localRepoPath, "push", "container-use", "--force", currentBranch)
	if err != nil {
		return "", err
	}

	// create worktree, accomodating past partial failures where the branch pushed but the worktree wasn't created
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

	if err := c.applyUncommittedChanges(localRepoPath, worktreePath); err != nil {
		return "", fmt.Errorf("failed to apply uncommitted changes: %w", err)
	}

	_, err = runGitCommand(localRepoPath, "fetch", "container-use", c.BranchName())
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
	slog.Info(fmt.Sprintf("[%s] $ git %s", dir, strings.Join(args, " ")))

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

func (env *Environment) propagateToWorktree(ctx context.Context, name, explanation string) error {
	worktreePath, err := env.GetWorktreePath()
	if err != nil {
		return err
	}

	_, err = env.container.Directory(env.Workdir).Export(
		ctx,
		worktreePath,
		dagger.DirectoryExportOpts{Wipe: true},
	)
	if err != nil {
		return err
	}

	slog.Info("Saving environment")
	if err := env.save(worktreePath); err != nil {
		return err
	}

	slog.Info("Committing changes to worktree")
	if err := env.commitWorktreeChanges(worktreePath, name, explanation); err != nil {
		return fmt.Errorf("failed to commit worktree changes: %w", err)
	}

	localRepoPath, err := filepath.Abs(env.Source)
	if err != nil {
		return err
	}

	slog.Info("Fetching container-use remote in source repository")
	_, err = runGitCommand(localRepoPath, "fetch", "container-use", env.BranchName())
	return err
}

func (s *Environment) commitWorktreeChanges(worktreePath, name, explanation string) error {
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
func (s *Environment) addNonBinaryFiles(worktreePath string) error {
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

		indexStatus := line[0]
		workTreeStatus := line[1]
		fileName := strings.TrimSpace(line[2:])
		if fileName == "" {
			continue
		}

		if s.shouldSkipFile(fileName) {
			continue
		}

		switch {
		case indexStatus == '?' && workTreeStatus == '?':
			// ?? = untracked files or directories
			if strings.HasSuffix(fileName, "/") {
				// Untracked directory - traverse and add non-binary files
				dirName := strings.TrimSuffix(fileName, "/")
				if err := s.addFilesFromUntrackedDirectory(worktreePath, dirName); err != nil {
					return err
				}
			} else {
				// Untracked file - add if not binary
				if !s.isBinaryFile(worktreePath, fileName) {
					_, err = runGitCommand(worktreePath, "add", fileName)
					if err != nil {
						return err
					}
				}
			}
		case indexStatus == 'A':
			// A = already staged, skip
			continue
		case indexStatus == 'D' || workTreeStatus == 'D':
			// D = deleted files (always stage deletion)
			_, err = runGitCommand(worktreePath, "add", fileName)
			if err != nil {
				return err
			}
		default:
			// M, R, C and other statuses - add if not binary
			if !s.isBinaryFile(worktreePath, fileName) {
				_, err = runGitCommand(worktreePath, "add", fileName)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (s *Environment) shouldSkipFile(fileName string) bool {
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
		"venv/", ".venv/", "env/", ".env/",
		"target/", "build/", "dist/", ".next/",
		"*.tmp", "*.temp", "*.cache", "*.log",
	}

	for _, pattern := range skipPatterns {
		if strings.Contains(lowerName, strings.ToLower(pattern)) {
			return true
		}
	}

	return false
}

func (c *Environment) applyUncommittedChanges(localRepoPath, worktreePath string) error {
	status, err := runGitCommand(localRepoPath, "status", "--porcelain")
	if err != nil {
		return err
	}

	if strings.TrimSpace(status) == "" {
		return nil
	}

	slog.Info("Applying uncommitted changes to worktree", "container-id", c.ID, "container-name", c.Name)

	patch, err := runGitCommand(localRepoPath, "diff", "HEAD")
	if err != nil {
		return err
	}

	if strings.TrimSpace(patch) != "" {
		cmd := exec.Command("git", "apply")
		cmd.Dir = worktreePath
		cmd.Stdin = strings.NewReader(patch)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to apply patch: %w", err)
		}
	}

	untrackedFiles, err := runGitCommand(localRepoPath, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return err
	}

	for _, file := range strings.Split(strings.TrimSpace(untrackedFiles), "\n") {
		if file == "" {
			continue
		}
		srcPath := filepath.Join(localRepoPath, file)
		destPath := filepath.Join(worktreePath, file)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		if err := exec.Command("cp", "-r", srcPath, destPath).Run(); err != nil {
			return fmt.Errorf("failed to copy untracked file %s: %w", file, err)
		}
	}

	return c.commitWorktreeChanges(worktreePath, "Copy uncommitted changes", "Applied uncommitted changes from local repository")
}

func (s *Environment) addFilesFromUntrackedDirectory(worktreePath, dirName string) error {
	dirPath := filepath.Join(worktreePath, dirName)

	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(worktreePath, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			if s.shouldSkipFile(relPath + "/") {
				return filepath.SkipDir
			}
			return nil
		}

		if s.shouldSkipFile(relPath) {
			return nil
		}

		if !s.isBinaryFile(worktreePath, relPath) {
			_, err = runGitCommand(worktreePath, "add", relPath)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *Environment) isBinaryFile(worktreePath, fileName string) bool {
	fullPath := filepath.Join(worktreePath, fileName)

	stat, err := os.Stat(fullPath)
	if err != nil {
		return true
	}

	if stat.IsDir() {
		return false
	}

	if stat.Size() > maxFileSizeForTextCheck {
		return true
	}

	file, err := os.Open(fullPath)
	if err != nil {
		slog.Error("Error opening file", "err", err)
		return true
	}
	defer file.Close()

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
