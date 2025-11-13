package exporter

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rxtx-hosting/flowlens/pkg/estimator"
)

type APIServer struct {
	apiKey string
	cache  map[string]estimator.ServerPlayerStats
	mu     sync.RWMutex
}

type metricsResponse struct {
	ServerID            string   `json:"server_id"`
	ActivePlayers       int      `json:"active_players"`
	UniqueIPs           []string `json:"unique_ips,omitempty"`
	SampleWindowSeconds int      `json:"sample_window_seconds"`
	TotalBytes          uint64   `json:"total_bytes"`
	Timestamp           string   `json:"timestamp"`
}

func NewAPIServer(apiKey string) *APIServer {
	return &APIServer{
		apiKey: apiKey,
		cache:  make(map[string]estimator.ServerPlayerStats),
	}
}

func (a *APIServer) UpdateStats(stats []estimator.ServerPlayerStats) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.cache = make(map[string]estimator.ServerPlayerStats)
	for _, stat := range stats {
		a.cache[stat.ServerID] = stat
	}
}

func (a *APIServer) StartServer(addr string) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	r.Use(a.authMiddleware())

	r.GET("/metrics/servers", a.handleGetAllServers)
	r.GET("/metrics/servers/:id", a.handleGetServer)

	return r.Run(addr)
}

func (a *APIServer) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "Bearer "+a.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		c.Next()
	}
}

func (a *APIServer) handleGetAllServers(c *gin.Context) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	response := make([]metricsResponse, 0, len(a.cache))
	for _, stat := range a.cache {
		response = append(response, a.statToResponse(stat))
	}

	c.JSON(http.StatusOK, gin.H{"servers": response})
}

func (a *APIServer) handleGetServer(c *gin.Context) {
	id := c.Param("id")

	a.mu.RLock()
	stat, exists := a.cache[id]
	a.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	c.JSON(http.StatusOK, a.statToResponse(stat))
}

func (a *APIServer) statToResponse(stat estimator.ServerPlayerStats) metricsResponse {
	return metricsResponse{
		ServerID:            stat.ServerID,
		ActivePlayers:       stat.ActivePlayers,
		UniqueIPs:           stat.UniqueIPs,
		SampleWindowSeconds: int(stat.SampleWindow.Seconds()),
		TotalBytes:          stat.TotalBytes,
		Timestamp:           stat.Timestamp.Format(time.RFC3339),
	}
}
