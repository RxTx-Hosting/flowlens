package estimator

import "time"

type ServerPlayerStats struct {
	ServerID      string
	ActivePlayers int
	UniqueIPs     []string
	TotalBytes    uint64
	SampleWindow  time.Duration
	Timestamp     time.Time
}
