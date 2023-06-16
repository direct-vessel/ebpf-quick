# ebpf-operator
ebpf-operator runs on Kubernetes, which is used to create a daemonset resource to run ebpf program on nodes of cluster.

## Usage
1、Build ebpf-operator

`make build`

2、Build the docker image (or push it to your image registry when the code has changes)

`make docker-build`

3、Deploy CR and ebpf-operator to your Kubernetes cluster

`kubectl apply -f deploy/ebpf-operator.yaml` 

4、Check ebpf-operator has successfully running on your Kubernetes cluster

`[root@VM-24-16-centos ebpf-operator]# kubectl get pods -n ebpf-operator-system
NAME                                                READY   STATUS    RESTARTS   AGE
ebpf-operator-controller-manager-664f694bcd-7cl7j   2/2     Running   0          4h43m`
