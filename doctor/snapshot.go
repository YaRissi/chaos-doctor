package main

import (
	"context"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type snapshot struct {
	namespace, app string
	deploy         *appsv1.Deployment
	replicaSets    []appsv1.ReplicaSet
	pods           []corev1.Pod
	service        *corev1.Service
	slices         []discoveryv1.EndpointSlice
	events         []corev1.Event
	configMaps     map[string]bool
	desired        desiredState
	requestErr     error
}

func (d *doctor) takeSnapshot(ctx context.Context) (*snapshot, error) {
	ns, apps, core := d.namespace, d.cs.AppsV1(), d.cs.CoreV1()
	s := &snapshot{namespace: ns, app: d.app, configMaps: map[string]bool{}}
	var err error

	if s.deploy, err = apps.Deployments(ns).Get(ctx, d.app, metav1.GetOptions{}); err != nil {
		d.ui.bad("I can't read deployment '%s' any more: %v", d.app, err)
		return nil, stopError{exitUnexaminable}
	}
	rsList, err := apps.ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, rs := range rsList.Items {
		if metav1.IsControlledBy(&rs, s.deploy) {
			s.replicaSets = append(s.replicaSets, rs)
		}
	}
	pods, err := core.Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, pod := range pods.Items {
		for i := range s.replicaSets {
			if metav1.IsControlledBy(&pod, &s.replicaSets[i]) {
				s.pods = append(s.pods, pod)
			}
		}
	}
	s.service, err = core.Services(ns).Get(ctx, d.app, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		s.service = nil
	} else if err != nil {
		return nil, err
	}
	endpointSlices, err := d.cs.DiscoveryV1().EndpointSlices(ns).List(ctx,
		metav1.ListOptions{LabelSelector: discoveryv1.LabelServiceName + "=" + d.app})
	if err != nil {
		return nil, err
	}
	s.slices = endpointSlices.Items
	events, err := core.Events(ns).List(ctx, metav1.ListOptions{FieldSelector: "type=Warning"})
	if err != nil {
		return nil, err
	}
	s.events = events.Items
	configMaps, err := core.ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	for _, cm := range configMaps.Items {
		s.configMaps[cm.Name] = true
	}
	s.desired = loadDesired(ctx, d.cs, s.deploy)
	if s.service != nil && len(s.service.Spec.Ports) > 0 {
		s.requestErr = d.requestThroughService(ctx, s.service)
	}
	return s, nil
}

func (d *doctor) requestThroughService(ctx context.Context, svc *corev1.Service) error {
	port := svc.Spec.Ports[0].Name
	if port == "" {
		port = strconv.Itoa(int(svc.Spec.Ports[0].Port))
	}
	_, err := d.cs.CoreV1().Services(svc.Namespace).ProxyGet("", svc.Name, port, "/", nil).DoRaw(ctx)
	return err
}
