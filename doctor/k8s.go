package main

import (
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

func desiredReplicas(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas == nil {
		return 1
	}
	return *dep.Spec.Replicas
}

func newestReplicaSet(s *snapshot) *appsv1.ReplicaSet {
	for i := range s.replicaSets {
		if s.replicaSets[i].Annotations[revisionAnnotation] == s.deploy.Annotations[revisionAnnotation] {
			return &s.replicaSets[i]
		}
	}
	return nil
}

func podReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func waitingState(pod *corev1.Pod) *corev1.ContainerStateWaiting {
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil
	}
	return pod.Status.ContainerStatuses[0].State.Waiting
}

func latestEvent(events []corev1.Event, match func(corev1.Event) bool) *corev1.Event {
	var latest *corev1.Event
	for i := range events {
		if match(events[i]) && (latest == nil || events[i].LastTimestamp.After(latest.LastTimestamp.Time)) {
			latest = &events[i]
		}
	}
	return latest
}

func latestWarning(events []corev1.Event, object string) string {
	if ev := latestEvent(events, func(ev corev1.Event) bool { return ev.InvolvedObject.Name == object }); ev != nil {
		return fmt.Sprintf("; latest warning: %s: %s", ev.Reason, ev.Message)
	}
	return ""
}

func labelString(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	slices.Sort(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

func rolloutUndo(s *snapshot) string {
	return fmt.Sprintf("kubectl -n %s rollout undo deployment/%s", s.namespace, s.app)
}
