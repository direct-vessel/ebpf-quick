# ebpf-quick

ebpf-quick is used to run eBPF programs on Kubernetes cluster. This project was inspired by [bpftools/kube-bpf](https://github.com/bpftools/kube-bpf)，the main differences are that it rewrites an ebpf-operator program by kubebuilder tool，and in future this project will support to run eBPF with Wasm sandbox on Kubernetes.

This project includes two part, one is ebpf-operator program, other is run-ebpf program. The ebpf-operator program is used to create a Daemonset resource to run run-ebpf program
on nodes of cluster. The run-ebpf program, as its name,  is used to run an ebpf program on nodes, and provides mapcollector to Prometheus for bpf map data.
