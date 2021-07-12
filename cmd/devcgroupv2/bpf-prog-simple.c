/**
 * build with:
 * clang -target bpf bpf-prog.c $(pkg-config --cflags libbpf) -O2 -c -g -o bpf-prog.o
 */

#include <linux/bpf.h>
#include <linux/version.h>
#include <bpf/bpf_helpers.h>

struct access_pattern {
	__u8 type;
	__u32 major;
	__u32 minor;
} __attribute__((packed));

struct {
	__uint(type, BPF_MAP_TYPE_HASH);
	__uint(max_entries, 500);
	__type(key, struct access_pattern);
	 __type(value, __u8);
} hash_map SEC(".maps");

SEC("cgroup/dev")
int bpf_prog1(struct bpf_cgroup_dev_ctx *ctx)
{

	struct access_pattern key = { };
	key.major = ctx->major;
	key.minor = ctx->minor;
	int type = ctx->access_type & 0xffff;

	switch (type) {
	case BPF_DEVCG_DEV_BLOCK:
		key.type = 'b';
		break;
	case BPF_DEVCG_DEV_CHAR:
		key.type = 'c';
		break;
	default:
		return 1;
	}
	void *result = bpf_map_lookup_elem(&hash_map, &key);
	if (result == NULL) {
		return 0;
	}
	return 1;
}

char _license[] SEC("license") = "GPL";
__u32 _version SEC("version") = LINUX_VERSION_CODE;
