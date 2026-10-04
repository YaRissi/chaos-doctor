// Package cluster reads everything the diagnosis needs from the Kubernetes API, once per round.
package cluster

import (
	"context"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type Snapshot struct {
	Namespace, App string
	Deployment     *appsv1.Deployment
	ReplicaSets    []appsv1.ReplicaSet
	Pods           []corev1.Pod
	Service        *corev1.Service
	EndpointSlices []discoveryv1.EndpointSlice
	Events         []corev1.Event
	ConfigMaps     map[string]bool
	Desired        Desired
	RequestErr     error
}

func Take(ctx context.Context, cs kubernetes.Interface, namespace, app string) (*Snapshot, error) {
	apps, core := cs.AppsV1(), cs.CoreV1()
	s := &Snapshot{Namespace: namespace, App: app, ConfigMaps: map[string]bool{}}
	var err error

	if s.Deployment, err = apps.Deployments(namespace).Get(ctx, app, metav1.GetOptions{}); err != nil {
		return nil, err
	}
	rsList, err := apps.ReplicaSets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, rs := range rsList.Items {
		if metav1.IsControlledBy(&rs, s.Deployment) {
			s.ReplicaSets = append(s.ReplicaSets, rs)
		}
	}
	pods, err := core.Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		for i := range s.ReplicaSets {
			if metav1.IsControlledBy(&pod, &s.ReplicaSets[i]) {
				s.Pods = append(s.Pods, pod)
			}
		}
	}
	s.Service, err = core.Services(namespace).Get(ctx, app, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		s.Service = nil
	} else if err != nil {
		return nil, err
	}
	slices, err := cs.DiscoveryV1().EndpointSlices(namespace).List(ctx,
		metav1.ListOptions{LabelSelector: discoveryv1.LabelServiceName + "=" + app})
	if err != nil {
		return nil, err
	}
	s.EndpointSlices = slices.Items
	events, err := core.Events(namespace).List(ctx, metav1.ListOptions{FieldSelector: "type=Warning"})
	if err != nil {
		return nil, err
	}
	s.Events = events.Items
	configMaps, err := core.ConfigMaps(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, cm := range configMaps.Items {
		s.ConfigMaps[cm.Name] = true
	}
	s.Desired = LoadDesired(ctx, cs, s.Deployment)
	if s.Service != nil && len(s.Service.Spec.Ports) > 0 {
		s.RequestErr = requestThroughService(ctx, cs, s.Service)
	}
	return s, nil
}

func requestThroughService(ctx context.Context, cs kubernetes.Interface, svc *corev1.Service) error {
	port := svc.Spec.Ports[0].Name
	if port == "" {
		port = strconv.Itoa(int(svc.Spec.Ports[0].Port))
	}
	_, err := cs.CoreV1().Services(svc.Namespace).ProxyGet("", svc.Name, port, "/", nil).DoRaw(ctx)
	return err
}
