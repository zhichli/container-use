package main

import (
	"context"
	"errors"
	"fmt"
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

func ForkContainer(sourceID string) *Container {
	source := GetContainer(sourceID)
	if source == nil {
		return nil
	}

	id := uuid.New().String()
	container := &Container{
		ID:    id,
		Image: source.Image,
		state: source.state,
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
