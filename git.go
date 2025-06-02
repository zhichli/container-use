package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	_, err = runGitCommand(localRepoPath, "fetch", "container-use")
	if err != nil {
		return "", err
	}

	currentBranch, err := runGitCommand(localRepoPath, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	currentBranch = strings.TrimSpace(currentBranch)

	_, err = runGitCommand(localRepoPath, "push", "container-use", currentBranch)
	if err != nil {
		return "", err
	}

	_, err = runGitCommand(cuRepoPath, "worktree", "add", "-b", c.BranchName(), worktreePath, currentBranch)
	if err != nil {
		return "", err
	}

	_, err = runGitCommand(localRepoPath, "branch", "--track", c.BranchName(), fmt.Sprintf("container-use/%s", c.BranchName()))
	if err != nil {
		return "", err
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

	_, err = runGitCommand(localRepoPath, "clone", "--bare", localRepoPath, cuRepoPath)
	if err != nil {
		return "", err
	}
	_, err = runGitCommand(localRepoPath, "remote", "add", "container-use", cuRepoPath)
	if err != nil {
		return "", err
	}
	return "", nil
}

func runGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
