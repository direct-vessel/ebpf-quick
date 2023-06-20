/*
Copyright 2023 The Kubernetes authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"path"

	"github.com/go-logr/logr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bpfv1 "ebpf-operator/api/v1"
)

// BPFReconciler reconciles a BPF object
type BPFReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the BPF object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *BPFReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	// Load the BPF Resource by name.
	ebpf := &bpfv1.BPF{}
	log.Info("fetching BPF Resource")
	if err := r.Get(ctx, req.NamespacedName, ebpf); err != nil {
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		if errors.IsNotFound(err) {
			log.Info(" ebpf not found")
			return reconcile.Result{}, nil
		}

		log.Error(err, "unable to fetch ebpf")
		return ctrl.Result{}, err
	}

	// name of our custom finalizer
	ebpfFinalizer := "ebpf.bpf.cloud/finalizer"

	if ebpf.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !ContainsString(ebpf.ObjectMeta.Finalizers, ebpfFinalizer) {
			ebpf.ObjectMeta.Finalizers = append(ebpf.ObjectMeta.Finalizers, ebpfFinalizer)
			if err := r.Update(ctx, ebpf); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.createDaemonSet(ctx, log, ebpf); err != nil {
				return ctrl.Result{}, nil
			}
		}
	} else {
		// The object is being deleted
		if ContainsString(ebpf.ObjectMeta.Finalizers, ebpfFinalizer) {
			ebpf.ObjectMeta.Finalizers = RemoveString(ebpf.ObjectMeta.Finalizers, ebpfFinalizer)
			if err := r.Update(ctx, ebpf); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *BPFReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&bpfv1.BPF{}).
		Complete(r)
}

func (r *BPFReconciler) createDaemonSet(ctx context.Context, log logr.Logger, ebpf *bpfv1.BPF) error {
	bpfProgramAbsolutePath := "/bpf"
	if ebpf.ObjectMeta.Labels == nil {
		ebpf.ObjectMeta.Labels = map[string]string{}
	}

	ebpf.ObjectMeta.Labels["bpf.sh/bpf-origin-uid"] = string(ebpf.ObjectMeta.UID)

	appName := fmt.Sprintf("bpf-%s", ebpf.Name)
	daemonset := &appsv1.DaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        appName,
			Namespace:   ebpf.Namespace,
			Labels:      ebpf.ObjectMeta.Labels,
			Annotations: ebpf.ObjectMeta.Annotations,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": appName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": appName,
					},
				},
				Spec: corev1.PodSpec{
					HostNetwork: true, // --net="host" // todos > this means two BPF resources cannot run together (since the port will be occupied)
					HostPID:     true, // --pid="host"
					// NodeSelector: map[string]string{}, // todos > node filtering/selection?
					Containers: []corev1.Container{
						{
							Name:  appName,
							Image: ebpf.Spec.Runner,
							Args: []string{
								path.Join(bpfProgramAbsolutePath, ebpf.Spec.Program.ValueFrom.ConfigMapKeyRef.Key),
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 9387,
								},
							},
							ImagePullPolicy: "IfNotPresent",
							SecurityContext: &corev1.SecurityContext{
								Privileged: pointer.BoolPtr(true),
							},
							Env: []corev1.EnvVar{
								{
									Name: "NODENAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								corev1.VolumeMount{
									Name:      "sys",
									MountPath: "/sys",
									ReadOnly:  true,
								},
								corev1.VolumeMount{
									Name:      "program",
									MountPath: bpfProgramAbsolutePath,
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						corev1.Volume{
							Name: "sys",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/sys",
								},
							},
						},
						corev1.Volume{
							Name: "program",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: ebpf.Spec.Program.ValueFrom.ConfigMapKeyRef.Name,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// set the owner so that garbage collection can kicks in
	if err := ctrl.SetControllerReference(ebpf, daemonset, r.Scheme); err != nil {
		log.Error(err, "unable to set ownerReference from BPF to Daemonset")
		return err
	}

	// 创建daemonset
	log.Info("start create daemonset")
	if err := r.Create(ctx, daemonset); err != nil {
		log.Error(err, "create daemonset error")
		return err
	}

	log.Info("create daemonset success")

	return nil

}

func ContainsString(slice []string, s string) bool {

	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}

	return
}
