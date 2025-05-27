package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"dagger.io/dagger"
	"github.com/google/uuid"
)

type Version int

type Revision struct {
	Version     Version
	Name        string
	Explanation string
	CreatedAt   time.Time

	state *dagger.Container
}

type History []*Revision

func (h History) Latest() *Revision {
	if len(h) == 0 {
		return nil
	}
	return h[len(h)-1]
}

func (h History) LatestVersion() Version {
	latest := h.Latest()
	if latest == nil {
		return 0
	}
	return latest.Version
}

func (h History) Get(version Version) *Revision {
	for _, revision := range h {
		if revision.Version == version {
			return revision
		}
	}
	return nil
}

type Container struct {
	ID      string
	Image   string
	History History

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

func CreateContainer(explanation, image string) (*Container, error) {
	id := uuid.New().String()
	container := &Container{
		ID:    id,
		Image: image,
	}
	err := container.apply(context.Background(), "Create container from "+image, explanation, dag.Container().
		From(image).
		WithWorkdir("/workdir"))
	if err != nil {
		return nil, err
	}
	containers[container.ID] = container
	return container, nil
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

func (s *Container) apply(ctx context.Context, name, explanation string, newState *dagger.Container) error {
	if _, err := newState.Sync(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	version := s.History.LatestVersion() + 1
	s.state = newState
	s.History = append(s.History, &Revision{
		Version:     version,
		Name:        name,
		Explanation: explanation,
		CreatedAt:   time.Now(),
		state:       newState,
	})

	return saveState(s)
}

func (s *Container) Run(ctx context.Context, explanation, command, shell string) (string, error) {
	newState := s.state.WithExec([]string{shell, "-c", command})
	stdout, err := newState.Stdout(ctx)
	if err != nil {
		var exitErr *dagger.ExecError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("command failed with exit code %d.\nstdout: %s\nstderr: %s", exitErr.ExitCode, exitErr.Stdout, exitErr.Stderr), nil
		}
		return "", err
	}
	if err := s.apply(ctx, "Run "+command, explanation, newState); err != nil {
		return "", err
	}

	return stdout, nil
}

func (s *Container) Revert(ctx context.Context, explanation string, version Version) error {
	revision := s.History.Get(version)
	if revision == nil {
		return errors.New("no revisions found")
	}
	if err := s.apply(ctx, "Revert to "+revision.Name, explanation, revision.state); err != nil {
		return err
	}
	return nil
}
