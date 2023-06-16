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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
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
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=bpf.cloud,resources=bpfs/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

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
	log := log.FromContext(ctx).WithValues("ebpf", req.NamespacedName)
	// Load the BPF Resource by name.
	var ebpf bpfv1.BPF
	log.Info("fetching BPF Resource")
	if err := r.Get(ctx, req.NamespacedName, &ebpf); err != nil {
		log.Error(err, "unable to fetch ebpf")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Clean Up old Daemonset and Service which had been owned by BPF Resource.
	if err := r.cleanupOwnedResources(ctx, log, &ebpf); err != nil {
		log.Error(err, "failed to clean up old daemonset and sevice resources for this BPF")
		return ctrl.Result{}, err
	}

	// Create or Update daemonset and service object which match bpf.Spec.
	if err := createOrUpdateDaemonSet(ctx, log, r, &ebpf); err != nil {
		return ctrl.Result{}, err
	}
	if err := createOrUpdateService(ctx, log, r, &ebpf); err != nil {
		return ctrl.Result{}, err
	}

	return reconcile.Result{}, nil

}

var (
	resourceOwnerKey = ".metadata.controller"
	apiGVStr         = bpfv1.GroupVersion.String()
)

// SetupWithManager sets up the controller with the Manager.
func (r *BPFReconciler) SetupWithManager(mgr ctrl.Manager) error {

	ctx := context.Background()

	// add resourceOwnerKey index to daemonset and service object which ebpf resource owns
	if err := mgr.GetFieldIndexer().IndexField(ctx, &appsv1.DaemonSet{}, resourceOwnerKey, func(object client.Object) []string {
		// grab the daemonset object, extract the owner...
		daemonset := object.(*appsv1.DaemonSet)
		owner := metav1.GetControllerOf(daemonset)
		if owner == nil {
			return nil
		}
		// ...make sure it's a BPF...
		if owner.APIVersion != apiGVStr || owner.Kind != "BPF" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(ctx, &corev1.Service{}, resourceOwnerKey, func(object client.Object) []string {
		// grab the service object, extract the owner...
		service := object.(*corev1.Service)
		owner := metav1.GetControllerOf(service)
		if owner == nil {
			return nil
		}
		// ...make sure it's a BPF...
		if owner.APIVersion != apiGVStr || owner.Kind != "BPF" {
			return nil
		}

		// ...and if so, return it
		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&bpfv1.BPF{}).
		Complete(r)
}

// cleanupOwnedResources will delete any existing daemonset and service resources that
// were created for the given BPF that no longer match the
// BPF.Name field.
func (r *BPFReconciler) cleanupOwnedResources(ctx context.Context, log logr.Logger, ebpf *bpfv1.BPF) error {
	log.Info("finding existing daemonset and service for BPF resource")

	// List all daemonset resources owned by this ebpf
	var daemonsets appsv1.DaemonSetList
	if err := r.List(ctx, &daemonsets, client.InNamespace(ebpf.Namespace), client.MatchingFields(map[string]string{resourceOwnerKey: ebpf.Name})); err != nil {
		return err
	}

	// Delete daemonset if the daemonset name doesn't match ebpf.Name
	for _, daemonset := range daemonsets.Items {
		if daemonset.Name == ebpf.Name {
			// If this daemonset's name matches the one on the BPF resource
			// then do not delete it.
			continue
		}

		// Delete old daemonset object which doesn't match ebpf.name.
		if err := r.Delete(ctx, &daemonset); err != nil {
			log.Error(err, "failed to delete daemonset resource")
			return err
		}

		log.Info("delete daemonset resource: " + daemonset.Name)
		//r.Recorder.Eventf(ebpf, corev1.EventTypeNormal, "Deleted", "Deleted daemonset %q", daemonset.Name)
	}

	// List all service resources owned by this ebpf
	var services corev1.ServiceList
	if err := r.List(ctx, &services, client.InNamespace(ebpf.Namespace), client.MatchingFields(map[string]string{resourceOwnerKey: ebpf.Name})); err != nil {
		return err
	}

	// Delete daemonset if the daemonset name doesn't match ebpf.Name
	for _, service := range services.Items {
		if service.Name == ebpf.Name {
			// If this daemonset's name matches the one on the BPF resource
			// then do not delete it.
			continue
		}

		// Delete old daemonset object which doesn't match BPF.name.
		if err := r.Delete(ctx, &service); err != nil {
			log.Error(err, "failed to delete service resource")
			return err
		}

		log.Info("delete service resource: " + service.Name)
		r.Recorder.Eventf(ebpf, corev1.EventTypeNormal, "Deleted", "Deleted service %q", service.Name)
	}

	return nil
}

func createOrUpdateDaemonSet(ctx context.Context, log logr.Logger, r *BPFReconciler, ebpf *bpfv1.BPF) error {
	const bpfProgramAbsolutePath = "/bpf"
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

func createOrUpdateService(ctx context.Context, log logr.Logger, r *BPFReconciler, ebpf *bpfv1.BPF) error {
	const bpfProgramAbsolutePath = "/bpf"
	appName := fmt.Sprintf("bpf-%s", ebpf.Name)
	if ebpf.ObjectMeta.Labels == nil {
		ebpf.ObjectMeta.Labels = map[string]string{}
	}
	ebpf.ObjectMeta.Labels["app"] = appName
	ebpf.ObjectMeta.Labels["bpf.sh/bpf-origin-uid"] = string(ebpf.ObjectMeta.UID)
	ebpf.ObjectMeta.Annotations["prometheus.io/scrape"] = "true"
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("bpf-%s", ebpf.Name),
			Namespace:   ebpf.Namespace,
			Labels:      ebpf.ObjectMeta.Labels,
			Annotations: ebpf.ObjectMeta.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				corev1.ServicePort{
					Name: "bpf",
					Port: 9387,
				},
			},
			Selector: map[string]string{
				"app": appName,
			},
			Type: "ClusterIP",
		},
	}

	// set the owner so that garbage collection can kicks in
	if err := ctrl.SetControllerReference(ebpf, service, r.Scheme); err != nil {
		log.Error(err, "unable to set ownerReference from BPF to Service")
		return err
	}

	// end of ctrl.CreateOrUpdate
	log.Info("create service success")

	return nil

}
