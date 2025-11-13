package estimator

import (
	"encoding/binary"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rxtx-hosting/flowlens/pkg/docker"
	"github.com/rxtx-hosting/flowlens/pkg/ebpf"
)

type Estimator struct {
	activityThreshold   time.Duration
	minPacketsThreshold uint64
	minBytesThreshold   uint64
	serverMap           map[int]string
}

func NewEstimator(activityThreshold time.Duration, minPackets, minBytes uint64) *Estimator {
	return &Estimator{
		activityThreshold:   activityThreshold,
		minPacketsThreshold: minPackets,
		minBytesThreshold:   minBytes,
		serverMap:           make(map[int]string),
	}
}

func (e *Estimator) UpdateServerMap(servers []docker.ServerMetadata) {
	newMap := make(map[int]string)
	for _, srv := range servers {
		newMap[srv.GamePort] = srv.ServerID
	}
	e.serverMap = newMap
}

func (e *Estimator) EstimatePlayers(flows map[ebpf.FlowKey]ebpf.FlowInfo) []ServerPlayerStats {
	bootTime, err := getBootTime()
	if err != nil {
		return []ServerPlayerStats{}
	}

	nowUnix := time.Now().UnixNano()
	nowSinceBoot := uint64(nowUnix - bootTime)
	cutoff := nowSinceBoot - uint64(e.activityThreshold.Nanoseconds())

	portFlows := make(map[int]map[string]uint64)

	var totalFlows, timeFiltered, thresholdFiltered, portFiltered, passed int
	var sampleCount int

	for key, info := range flows {
		totalFlows++

		if info.LastSeen < cutoff {
			timeFiltered++
			continue
		}

		ip := uint32ToIP(key.SrcIP)
		port := int(key.DstPort)

		if info.Packets < e.minPacketsThreshold || info.Bytes < e.minBytesThreshold {
			thresholdFiltered++
			if sampleCount < 5 {
				slog.Debug("Flow filtered", "ip", ip, "port", port, "packets", info.Packets, "bytes", info.Bytes, "reason", "below threshold")
				sampleCount++
			}
			continue
		}

		if _, exists := e.serverMap[port]; !exists {
			portFiltered++
			continue
		}

		if portFlows[port] == nil {
			portFlows[port] = make(map[string]uint64)
		}

		portFlows[port][ip] += info.Bytes
		if passed < 5 {
			slog.Debug("Flow passed", "ip", ip, "port", port, "packets", info.Packets, "bytes", info.Bytes)
		}
		passed++
	}

	slog.Debug("Flow filtering complete", "total", totalFlows, "timeFiltered", timeFiltered, "thresholdFiltered", thresholdFiltered, "portFiltered", portFiltered, "passed", passed, "minPackets", e.minPacketsThreshold, "minBytes", e.minBytesThreshold)

	stats := make([]ServerPlayerStats, 0, len(portFlows))

	for port, ipMap := range portFlows {
		serverID := e.serverMap[port]
		uniqueIPs := make([]string, 0, len(ipMap))
		var totalBytes uint64

		for ip, bytes := range ipMap {
			uniqueIPs = append(uniqueIPs, ip)
			totalBytes += bytes
		}

		slog.Debug("Server stats", "id", serverID, "port", port, "players", len(uniqueIPs), "totalBytes", totalBytes)

		stats = append(stats, ServerPlayerStats{
			ServerID:      serverID,
			ActivePlayers: len(uniqueIPs),
			UniqueIPs:     uniqueIPs,
			TotalBytes:    totalBytes,
			SampleWindow:  e.activityThreshold,
			Timestamp:     time.Now(),
		})
	}

	return stats
}

func uint32ToIP(ip uint32) string {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, ip)
	return net.IP(buf).String()
}

func getBootTime() (int64, error) {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0, err
	}

	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "btime ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				btime, err := strconv.ParseInt(fields[1], 10, 64)
				if err != nil {
					return 0, err
				}
				return btime * 1e9, nil
			}
		}
	}

	return 0, os.ErrNotExist
}
