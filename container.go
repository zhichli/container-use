package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"dagger.io/dagger"
	"github.com/google/uuid"
)

const (
	AlpineImage = "alpine:3.21.3@sha256:a8560b36e8b8210634f77d9f7f9efd7ffa463e380b75e2e74aff4511df3ef88c"
)

type Version int

type Revision struct {
	Version     Version   `json:"version"`
	Name        string    `json:"name"`
	Explanation string    `json:"explanation"`
	CreatedAt   time.Time `json:"created_at"`

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
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Image   string  `json:"image"`
	Workdir string  `json:"workdir"`
	History History `json:"history"`

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

func CreateContainer(name, explanation, image, workdir string) (*Container, error) {
	container := &Container{
		ID:      uuid.New().String(),
		Name:    name,
		Image:   image,
		Workdir: workdir,
	}
	err := container.apply(context.Background(), "Create container from "+image, explanation, dag.Container().
		From(image).
		WithWorkdir(workdir).
		WithDirectory(".", dag.Directory())) // Force workdir to exist

	if err != nil {
		return nil, err
	}
	containers[container.ID] = container
	return container, nil
}

func GetContainer(idOrName string) *Container {
	if container, ok := containers[idOrName]; ok {
		return container
	}
	for _, container := range containers {
		if container.Name == idOrName {
			return container
		}
	}
	return nil
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

func (s *Container) Run(ctx context.Context, explanation, command, shell string, useEntrypoint bool) (string, error) {
	args := []string{}
	if command != "" {
		args = []string{shell, "-c", command}
	}
	newState := s.state.WithExec(args, dagger.ContainerWithExecOpts{
		UseEntrypoint: useEntrypoint,
	})
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

type EndpointMapping struct {
	Internal string `json:"internal"`
	External string `json:"external"`
}

type EndpointMappings map[int]*EndpointMapping

func (s *Container) RunBackground(ctx context.Context, explanation, command, shell string, ports []int, useEntrypoint bool) (EndpointMappings, error) {
	args := []string{}
	if command != "" {
		args = []string{shell, "-c", command}
	}
	serviceState := s.state

	// Expose ports
	for _, port := range ports {
		serviceState = serviceState.WithExposedPort(port, dagger.ContainerWithExposedPortOpts{
			Protocol:    dagger.NetworkProtocolTcp,
			Description: fmt.Sprintf("Port %d", port),
		})
	}

	// Start the service
	svc, err := serviceState.AsService(dagger.ContainerAsServiceOpts{
		Args:          args,
		UseEntrypoint: useEntrypoint,
	}).Start(context.Background())
	if err != nil {
		var exitErr *dagger.ExecError
		if errors.As(err, &exitErr) {
			return nil, fmt.Errorf("command failed with exit code %d.\nstdout: %s\nstderr: %s", exitErr.ExitCode, exitErr.Stdout, exitErr.Stderr)
		}
		return nil, err
	}

	endpoints := EndpointMappings{}
	hostForwards := []dagger.PortForward{}

	for _, port := range ports {
		endpoints[port] = &EndpointMapping{}
		hostForwards = append(hostForwards, dagger.PortForward{
			Backend:  port,
			Frontend: rand.Intn(1000) + 5000,
			Protocol: dagger.NetworkProtocolTcp,
		})
	}

	// Expose ports on the host
	tunnel, err := dag.Host().Tunnel(svc, dagger.HostTunnelOpts{Ports: hostForwards}).Start(context.Background())
	if err != nil {
		return nil, err
	}

	// Retrieve endpoints
	for _, forward := range hostForwards {
		externalEndpoint, err := tunnel.Endpoint(ctx, dagger.ServiceEndpointOpts{
			Port: forward.Frontend,
		})
		if err != nil {
			return nil, err
		}

		endpoints[forward.Backend].External = externalEndpoint
	}
	for port, endpoint := range endpoints {
		internalEndpoint, err := svc.Endpoint(ctx, dagger.ServiceEndpointOpts{
			Port: port,
		})
		if err != nil {
			return nil, err
		}
		endpoint.Internal = internalEndpoint
	}

	return endpoints, nil
}

func (s *Container) SetEnv(ctx context.Context, explanation string, envs []string) error {
	state := s.state
	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable: %s", env)
		}
		state = state.WithEnvVariable(parts[0], parts[1])
	}
	return s.apply(ctx, "Set env "+strings.Join(envs, ", "), explanation, state)
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

func (s *Container) Fork(ctx context.Context, explanation, name string, version *Version) (*Container, error) {
	revision := s.History.Latest()
	if version != nil {
		revision = s.History.Get(*version)
	}
	if revision == nil {
		return nil, errors.New("version not found")
	}

	forkedContainer := &Container{
		ID:    uuid.New().String(),
		Name:  name,
		Image: s.Image,
	}
	if err := forkedContainer.apply(ctx, "Fork from "+s.Name, explanation, revision.state); err != nil {
		return nil, err
	}
	containers[forkedContainer.ID] = forkedContainer
	return forkedContainer, nil
}
