package docker

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
)

type Client struct {
	cli        *client.Client
	labels     map[string]string
	idSource   string
	portEnvVar string
}

func NewClient(labels map[string]string, idSource, portEnvVar string) (*Client, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Client{
		cli:        cli,
		labels:     labels,
		idSource:   idSource,
		portEnvVar: portEnvVar,
	}, nil
}

func (c *Client) Close() error {
	return c.cli.Close()
}

func (c *Client) DiscoverGameServers(ctx context.Context) ([]ServerMetadata, error) {
	filterArgs := filters.NewArgs()
	for key, value := range c.labels {
		filterArgs.Add("label", fmt.Sprintf("%s=%s", key, value))
	}
	filterArgs.Add("status", "running")

	containers, err := c.cli.ContainerList(ctx, container.ListOptions{
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	servers := make([]ServerMetadata, 0, len(containers))
	now := time.Now()

	for _, ctr := range containers {
		inspect, err := c.cli.ContainerInspect(ctx, ctr.ID)
		if err != nil {
			continue
		}

		serverID := c.extractID(inspect)
		if serverID == "" {
			continue
		}

		gamePort := c.extractPort(inspect, ctr.Ports)
		if gamePort == 0 {
			continue
		}

		servers = append(servers, ServerMetadata{
			ServerID:      serverID,
			GamePort:      gamePort,
			ContainerID:   ctr.ID,
			ContainerName: strings.TrimPrefix(ctr.Names[0], "/"),
			LastUpdated:   now,
		})
	}

	return servers, nil
}

func (c *Client) extractID(inspect types.ContainerJSON) string {
	switch c.idSource {
	case "hostname":
		return inspect.Config.Hostname
	case "id":
		return inspect.ID
	case "name":
		return strings.TrimPrefix(inspect.Name, "/")
	default:
		if strings.HasPrefix(c.idSource, "label:") {
			labelKey := strings.TrimPrefix(c.idSource, "label:")
			if val, ok := inspect.Config.Labels[labelKey]; ok {
				return val
			}
		} else if strings.HasPrefix(c.idSource, "env:") {
			envKey := strings.TrimPrefix(c.idSource, "env:")
			for _, e := range inspect.Config.Env {
				if strings.HasPrefix(e, envKey+"=") {
					return strings.TrimPrefix(e, envKey+"=")
				}
			}
		}
		return inspect.Config.Hostname
	}
}

func (c *Client) extractPort(inspect types.ContainerJSON, ports []container.Port) int {
	if c.portEnvVar != "" {
		for _, e := range inspect.Config.Env {
			if strings.HasPrefix(e, c.portEnvVar+"=") {
				portStr := strings.TrimPrefix(e, c.portEnvVar+"=")
				if port, err := strconv.Atoi(portStr); err == nil && port > 0 && port <= 65535 {
					return port
				}
			}
		}
	}

	for _, port := range ports {
		if port.PublicPort > 0 && port.PublicPort <= 65535 {
			return int(port.PublicPort)
		}
	}

	return 0
}
