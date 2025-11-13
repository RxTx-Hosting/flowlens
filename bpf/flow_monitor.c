//go:build ignore

#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <linux/pkt_cls.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>

struct flow_key {
	__u32 src_ip;
	__u16 dst_port;
	__u8  proto;
	__u8  _pad;
};

struct flow_info {
	__u64 packets;
	__u64 bytes;
	__u64 last_seen_ns;
};

struct {
	__uint(type, BPF_MAP_TYPE_LRU_HASH);
	__uint(max_entries, 100000);
	__type(key, struct flow_key);
	__type(value, struct flow_info);
} flow_stats SEC(".maps");

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 1000);
	__type(key, __u16);
	__type(value, __u8);
} monitored_ports SEC(".maps");

SEC("tc")
int flow_monitor(struct __sk_buff *skb)
{
	void *data = (void *)(long)skb->data;
	void *data_end = (void *)(long)skb->data_end;

	struct ethhdr *eth = data;
	if ((void *)(eth + 1) > data_end)
		return TC_ACT_OK;

	if (eth->h_proto != bpf_htons(ETH_P_IP))
		return TC_ACT_OK;

	struct iphdr *ip = (void *)(eth + 1);
	if ((void *)(ip + 1) > data_end)
		return TC_ACT_OK;

	__u8 ihl = ip->ihl;
	if (ihl < 5)
		return TC_ACT_OK;

	struct flow_key key = {0};
	key.src_ip = ip->saddr;
	key.proto = ip->protocol;

	__u16 dst_port = 0;
	void *l4 = (void *)ip + (ihl * 4);

	if (ip->protocol == IPPROTO_TCP) {
		if (l4 + sizeof(struct tcphdr) > data_end)
			return TC_ACT_OK;
		struct tcphdr *tcp = l4;
		dst_port = bpf_ntohs(tcp->dest);
	} else if (ip->protocol == IPPROTO_UDP) {
		if (l4 + sizeof(struct udphdr) > data_end)
			return TC_ACT_OK;
		struct udphdr *udp = l4;
		dst_port = bpf_ntohs(udp->dest);
	} else {
		return TC_ACT_OK;
	}

	key.dst_port = dst_port;

	if (!bpf_map_lookup_elem(&monitored_ports, &dst_port))
		return TC_ACT_OK;

	struct flow_info *info = bpf_map_lookup_elem(&flow_stats, &key);
	if (info) {
		__sync_fetch_and_add(&info->packets, 1);
		__sync_fetch_and_add(&info->bytes, skb->len);
		info->last_seen_ns = bpf_ktime_get_ns();
	} else {
		struct flow_info new_info = {
			.packets = 1,
			.bytes = skb->len,
			.last_seen_ns = bpf_ktime_get_ns(),
		};
		bpf_map_update_elem(&flow_stats, &key, &new_info, BPF_ANY);
	}

	return TC_ACT_OK;
}

char __license[] SEC("license") = "GPL";
