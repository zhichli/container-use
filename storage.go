package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"dagger.io/dagger"
	"github.com/mitchellh/go-homedir"
)

func saveState(c *Container) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}

	stateDir, err := homedir.Expand(fmt.Sprintf("~/.config/container-use/%s", c.ID))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(stateDir, "container.json"), data, 0644); err != nil {
		return err
	}

	stateID, err := c.state.ID(context.Background())
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(stateDir, "state"), []byte(stateID), 0644); err != nil {
		return err
	}

	return nil
}

func loadState() (map[string]*Container, error) {
	stateDir, err := homedir.Expand("~/.config/container-use")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return nil, err
	}
	containers := map[string]*Container{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := entry.Name()
		stateFile := filepath.Join(stateDir, id, "container.json")
		data, err := os.ReadFile(stateFile)
		if err != nil {
			return nil, err
		}
		var c Container
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
		state := filepath.Join(stateDir, id, "state")
		data, err = os.ReadFile(state)
		if err != nil {
			return nil, err
		}
		c.state = dag.LoadContainerFromID(dagger.ContainerID(data))

		containers[id] = &c
	}
	return containers, nil
}
