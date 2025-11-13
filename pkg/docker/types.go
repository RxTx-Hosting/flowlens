package docker

import "time"

type ServerMetadata struct {
	ServerID      string
	GamePort      int
	ContainerID   string
	ContainerName string
	LastUpdated   time.Time
}
