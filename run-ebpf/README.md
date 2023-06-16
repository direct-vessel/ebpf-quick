# run-ebpf

run-ebpf is used to run an ebpf program on nodes, and provides mapcollector to Prometheus for bpf map data.

## Usage

1、Build the runner and other tools

```console
make
```

2、Build the docker image for runbpf (or push it to your image registry when the code has changes)

```console
make image
docker push direct-vessel/runbpf:test
```

3、Build BPF program and create  the BPF custom resources

```console
make examples
```

4、Deploy  the BPF custom resources for paccetti program. (Note: **only one ebpf program at one time** can be running on Kubernetes cluster.)

```console
kubectl apply -f pacchetti.yaml
```

5、Check the paccetti program's daemonset if has running on your Kubernetes cluster.  

```console
# kubectl get ds bpf-pacchetti-bpf
[root@VM-24-16-centos output]# kubectl get ds bpf-pacchetti-bpf
NAME                DESIRED   CURRENT   READY   UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
bpf-pacchetti-bpf   1         1         1       1            1           <none>          4h53m
```

6、Access runbpf's mapcollector for bpf map created by ebpf program. The paccetti program is used to collect  a pair of protocol number -> count.  See the wikipedia article on protocol numbers:  https://en.wikipedia.org/wiki/List_of_IP_protocol_numbers

```console
[root@VM-24-16-centos output]# curl http://127.0.0.1:9387/metrics
# HELP bpf_packets packets
# TYPE bpf_packets counter
bpf_packets{key="00006",node="VM-24-16-centos"} 4.396146e+06
```
