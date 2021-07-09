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
int bpf_prog1(struct bpf_cgroup_dev_ctx *ctx) {
    short type = ctx->access_type & 0xFFFF;
    short access = ctx->access_type >> 16;
    char fmt[] = "  %d:%d    \n";
    char *result = NULL;

    struct access_pattern key = {};

    switch (type) {
        case BPF_DEVCG_DEV_BLOCK:
            fmt[0] = 'b';
            key.type = 'b';
            break;
        case BPF_DEVCG_DEV_CHAR:
            fmt[0] = 'c';
            key.type = 'c';
            break;
        default:
            fmt[0] = '?';
            break;
    }
    key.major = ctx->major;
    key.minor = ctx->minor;

    if (access & BPF_DEVCG_ACC_READ) fmt[8] = 'r';

    if (access & BPF_DEVCG_ACC_WRITE) fmt[9] = 'w';

    if (access & BPF_DEVCG_ACC_MKNOD) fmt[10] = 'm';

    bpf_trace_printk(fmt, sizeof(fmt), ctx->major, ctx->minor);

    result = bpf_map_lookup_elem(&hash_map, &key);
    if (result == NULL) {
        return 0;
    }
    return 1;
}

char _license[] SEC("license") = "GPL";
__u32 _version SEC("version") = LINUX_VERSION_CODE;
