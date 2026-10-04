package cluster

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

func DesiredReplicas(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas == nil {
		return 1
	}
	return *dep.Spec.Replicas
}

func (s *Snapshot) NewestReplicaSet() *appsv1.ReplicaSet {
	for i := range s.ReplicaSets {
		if s.ReplicaSets[i].Annotations[revisionAnnotation] == s.Deployment.Annotations[revisionAnnotation] {
			return &s.ReplicaSets[i]
		}
	}
	return nil
}

func (s *Snapshot) PreviousHealthyReplicaSet() *appsv1.ReplicaSet {
	newest := s.NewestReplicaSet()
	var best *appsv1.ReplicaSet
	bestRevision := -1
	for i := range s.ReplicaSets {
		rs := &s.ReplicaSets[i]
		revision, err := strconv.Atoi(rs.Annotations[revisionAnnotation])
		if err == nil && rs != newest && rs.Status.ReadyReplicas > 0 && revision > bestRevision {
			best, bestRevision = rs, revision
		}
	}
	return best
}

func PodReady(p *corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func WaitingState(pod *corev1.Pod) *corev1.ContainerStateWaiting {
	if len(pod.Status.ContainerStatuses) == 0 {
		return nil
	}
	return pod.Status.ContainerStatuses[0].State.Waiting
}

func LatestEvent(events []corev1.Event, match func(corev1.Event) bool) *corev1.Event {
	var latest *corev1.Event
	for i := range events {
		if match(events[i]) && (latest == nil || events[i].LastTimestamp.After(latest.LastTimestamp.Time)) {
			latest = &events[i]
		}
	}
	return latest
}

func LatestWarning(events []corev1.Event, object string) string {
	if ev := LatestEvent(events, func(ev corev1.Event) bool { return ev.InvolvedObject.Name == object }); ev != nil {
		return fmt.Sprintf("; latest warning: %s: %s", ev.Reason, ev.Message)
	}
	return ""
}

func LabelString(m map[string]string) string {
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, k+"="+v)
	}
	slices.Sort(parts)
	return "{" + strings.Join(parts, ",") + "}"
}
