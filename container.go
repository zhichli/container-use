package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"dagger.io/dagger"
	"github.com/google/uuid"
)

type Container struct {
	ID    string
	Image string

	mu    sync.Mutex
	state *dagger.Container
}

var containers = map[string]*Container{}

func LoadContainers() error {
	ctr, err := loadState()
	if err != nil {
		return err
	}
	containers = ctr
	return nil
}

func CreateContainer(image string) *Container {
	id := uuid.New().String()
	container := &Container{
		ID:    id,
		Image: image,

		state: dag.Container().
			From(image).
			WithWorkdir("/workdir"),
	}
	containers[container.ID] = container
	if err := saveState(container); err != nil {
		panic(err)
	}
	return container
}

func GetContainer(id string) *Container {
	return containers[id]
}

func ListContainers() []*Container {
	ctr := make([]*Container, 0, len(containers))
	for _, container := range containers {
		ctr = append(ctr, container)
	}
	return ctr
}

func (s *Container) apply(ctx context.Context, newState *dagger.Container) error {
	if _, err := newState.Sync(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = newState

	return saveState(s)
}

func (s *Container) Run(ctx context.Context, command string, shell string) (string, error) {
	newState := s.state.WithExec([]string{shell, "-c", command})
	stdout, err := newState.Stdout(ctx)
	if err != nil {
		var exitErr *dagger.ExecError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("command failed with exit code %d.\nstdout: %s\nstderr: %s", exitErr.ExitCode, exitErr.Stdout, exitErr.Stderr), nil
		}
		return "", err
	}
	if err := s.apply(ctx, newState); err != nil {
		return "", err
	}

	return stdout, nil
}

func (s *Container) FileRead(ctx context.Context, targetFile string, shouldReadEntireFile bool, startLineOneIndexed int, endLineOneIndexedInclusive int) (string, error) {
	file, err := s.state.File(targetFile).Contents(ctx)
	if err != nil {
		return "", err
	}
	if shouldReadEntireFile {
		return string(file), err
	}

	lines := strings.Split(string(file), "\n")
	start := startLineOneIndexed - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		start = len(lines) - 1
	}
	end := endLineOneIndexedInclusive
	if end >= len(lines) {
		end = len(lines) - 1
	}
	if end < 0 {
		end = 0
	}
	return strings.Join(lines[start:end], "\n"), nil
}

func (s *Container) FileWrite(ctx context.Context, targetFile string, contents string) error {
	return s.apply(ctx, s.state.WithNewFile(targetFile, contents))
}

func (s *Container) FileDelete(ctx context.Context, targetFile string) error {
	return s.apply(ctx, s.state.WithoutFile(targetFile))
}

func (s *Container) FileList(ctx context.Context, path string) (string, error) {
	entries, err := s.state.Directory(path).Entries(ctx)
	if err != nil {
		return "", err
	}
	out := &strings.Builder{}
	for _, entry := range entries {
		fmt.Fprintf(out, "%s\n", entry)
	}
	return out.String(), nil
}

func urlToDirectory(url string) *dagger.Directory {
	switch {
	case strings.HasPrefix(url, "file://"):
		return dag.Host().Directory(url[len("file://"):])
	case strings.HasPrefix(url, "git://"):
		return dag.Git(url[len("git://"):]).Head().Tree()
	case strings.HasPrefix(url, "https://"):
		return dag.Git(url[len("https://"):]).Head().Tree()
	default:
		return dag.Host().Directory(url)
	}
}

func (s *Container) Upload(ctx context.Context, source string, target string) error {
	return s.apply(ctx, s.state.WithDirectory(target, urlToDirectory(source)))
}

func (s *Container) Download(ctx context.Context, source string, target string) error {
	if _, err := s.state.Directory(source).Export(ctx, target); err != nil {
		if strings.Contains(err.Error(), "not a directory") {
			if _, err := s.state.File(source).Export(ctx, target); err != nil {
				return err
			}
			return nil
		}
		return err
	}

	return nil
}

func (s *Container) Diff(ctx context.Context, source string, target string) (string, error) {
	sourceDir := urlToDirectory(source)
	targetDir := s.state.Directory(target)

	diff, err := dag.Container().From("alpine").
		WithMountedDirectory("/source", sourceDir).
		WithMountedDirectory("/target", targetDir).
		WithExec([]string{"diff", "-burN", "/source", "/target"}, dagger.ContainerWithExecOpts{
			Expect: dagger.ReturnTypeAny,
		}).
		Stdout(ctx)
	if err != nil {
		var exitErr *dagger.ExecError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("command failed with exit code %d.\nstdout: %s\nstderr: %s", exitErr.ExitCode, exitErr.Stdout, exitErr.Stderr), nil
		}
		return "", err
	}
	return diff, nil
}
