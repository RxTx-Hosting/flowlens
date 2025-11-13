package exporter

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rxtx-hosting/flowlens/pkg/estimator"
)

type PrometheusExporter struct {
	activePlayers *prometheus.GaugeVec
	totalBytes    *prometheus.GaugeVec
	cache         map[string]estimator.ServerPlayerStats
	mu            sync.RWMutex
}

func NewPrometheusExporter() *PrometheusExporter {
	activePlayers := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "flowlens_active_players",
			Help: "Number of active players on game server",
		},
		[]string{"server_id"},
	)

	totalBytes := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "flowlens_total_bytes",
			Help: "Total bytes transferred in sample window",
		},
		[]string{"server_id"},
	)

	prometheus.MustRegister(activePlayers)
	prometheus.MustRegister(totalBytes)

	return &PrometheusExporter{
		activePlayers: activePlayers,
		totalBytes:    totalBytes,
		cache:         make(map[string]estimator.ServerPlayerStats),
	}
}

func (p *PrometheusExporter) UpdateStats(stats []estimator.ServerPlayerStats) {
	p.mu.Lock()
	defer p.mu.Unlock()

	newCache := make(map[string]estimator.ServerPlayerStats)

	for _, stat := range stats {
		newCache[stat.ServerID] = stat
		p.activePlayers.WithLabelValues(stat.ServerID).Set(float64(stat.ActivePlayers))
		p.totalBytes.WithLabelValues(stat.ServerID).Set(float64(stat.TotalBytes))
	}

	for serverID := range p.cache {
		if _, exists := newCache[serverID]; !exists {
			p.activePlayers.DeleteLabelValues(serverID)
			p.totalBytes.DeleteLabelValues(serverID)
		}
	}

	p.cache = newCache
}

func (p *PrometheusExporter) StartServer(addr string) error {
	http.Handle("/metrics", promhttp.Handler())
	return http.ListenAndServe(addr, nil)
}
