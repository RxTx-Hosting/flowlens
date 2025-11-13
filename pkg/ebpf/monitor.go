package ebpf

import (
	"fmt"

	"github.com/rxtx-hosting/flowlens/pkg/docker"
	"github.com/vishvananda/netlink"
)

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -type flow_key -type flow_info flowMonitor ../../bpf/flow_monitor.c -- -I/usr/include -I/usr/include/x86_64-linux-gnu -O2 -g

type Monitor struct {
	objs      *flowMonitorObjects
	iface     string
	serverMap map[int]string
}

func NewMonitor(iface string) (*Monitor, error) {
	objs := &flowMonitorObjects{}
	if err := loadFlowMonitorObjects(objs, nil); err != nil {
		return nil, fmt.Errorf("failed to load eBPF objects: %w", err)
	}

	link, err := netlink.LinkByName(iface)
	if err != nil {
		objs.Close()
		return nil, fmt.Errorf("failed to get interface %s: %w", iface, err)
	}

	attrs := netlink.QdiscAttrs{
		LinkIndex: link.Attrs().Index,
		Handle:    netlink.MakeHandle(0xffff, 0),
		Parent:    netlink.HANDLE_CLSACT,
	}

	qdisc := &netlink.GenericQdisc{
		QdiscAttrs: attrs,
		QdiscType:  "clsact",
	}

	if err := netlink.QdiscReplace(qdisc); err != nil {
		objs.Close()
		return nil, fmt.Errorf("failed to setup clsact qdisc: %w", err)
	}

	filter := &netlink.BpfFilter{
		FilterAttrs: netlink.FilterAttrs{
			LinkIndex: link.Attrs().Index,
			Parent:    netlink.HANDLE_MIN_INGRESS,
			Handle:    netlink.MakeHandle(0, 1),
			Protocol:  3,
			Priority:  1,
		},
		Fd:           objs.FlowMonitor.FD(),
		Name:         "flow_monitor",
		DirectAction: true,
	}

	if err := netlink.FilterReplace(filter); err != nil {
		objs.Close()
		return nil, fmt.Errorf("failed to attach BPF filter: %w", err)
	}

	return &Monitor{
		objs:      objs,
		iface:     iface,
		serverMap: make(map[int]string),
	}, nil
}

func (m *Monitor) Close() error {
	if m.iface != "" {
		link, err := netlink.LinkByName(m.iface)
		if err == nil {
			filter := &netlink.BpfFilter{
				FilterAttrs: netlink.FilterAttrs{
					LinkIndex: link.Attrs().Index,
					Parent:    netlink.HANDLE_MIN_INGRESS,
					Handle:    netlink.MakeHandle(0, 1),
					Protocol:  3,
					Priority:  1,
				},
			}
			netlink.FilterDel(filter)
		}
	}

	if m.objs != nil {
		m.objs.Close()
	}
	return nil
}

func (m *Monitor) ReadFlows() (map[FlowKey]FlowInfo, error) {
	flows := make(map[FlowKey]FlowInfo)

	var key FlowKey
	var val FlowInfo

	iter := m.objs.FlowStats.Iterate()
	for iter.Next(&key, &val) {
		flows[key] = val
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate map: %w", err)
	}

	return flows, nil
}

func (m *Monitor) UpdateServers(servers []docker.ServerMetadata) error {
	newMap := make(map[int]string)
	newPorts := make(map[uint16]bool)

	for _, srv := range servers {
		newMap[srv.GamePort] = srv.ServerID
		newPorts[uint16(srv.GamePort)] = true
	}

	for port := range newPorts {
		var val uint8 = 1
		if err := m.objs.MonitoredPorts.Put(&port, &val); err != nil {
			return fmt.Errorf("failed to add port %d: %w", port, err)
		}
	}

	var oldPort uint16
	var oldVal uint8
	iter := m.objs.MonitoredPorts.Iterate()
	for iter.Next(&oldPort, &oldVal) {
		if !newPorts[oldPort] {
			if err := m.objs.MonitoredPorts.Delete(&oldPort); err != nil {
				return fmt.Errorf("failed to remove port %d: %w", oldPort, err)
			}
		}
	}

	m.serverMap = newMap
	return nil
}

func (m *Monitor) GetServerID(port int) (string, bool) {
	id, ok := m.serverMap[port]
	return id, ok
}

func (m *Monitor) GetServerMap() map[int]string {
	return m.serverMap
}
