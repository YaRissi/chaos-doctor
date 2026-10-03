package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func oomKilled(s *snapshot, pod *corev1.Pod) *finding {
	waiting := waitingState(pod)
	if waiting == nil || waiting.Reason != "CrashLoopBackOff" {
		return nil
	}
	last := pod.Status.ContainerStatuses[0].LastTerminationState.Terminated
	if last == nil || last.Reason != "OOMKilled" {
		return nil
	}
	return &finding{
		symptom:  fmt.Sprintf("Pod '%s' is crash-looping.", pod.Name),
		disease:  "oom-killed",
		cause:    "The newest pod template's memory limit is too low: the container is OOMKilled on start and crash-loops.",
		evidence: fmt.Sprintf("lastState.terminated.reason = OOMKilled; memory limit: %s", pod.Spec.Containers[0].Resources.Limits.Memory()),
		suggest:  rolloutUndo(s),
	}
}
