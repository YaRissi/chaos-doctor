package diseases

import (
	"fmt"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

func oomKilled(s *cluster.Snapshot, pod *corev1.Pod) *Finding {
	waiting := cluster.WaitingState(pod)
	if waiting == nil || waiting.Reason != "CrashLoopBackOff" {
		return nil
	}
	last := pod.Status.ContainerStatuses[0].LastTerminationState.Terminated
	if last == nil || last.Reason != "OOMKilled" {
		return nil
	}
	return &Finding{
		Symptom:  fmt.Sprintf("Pod '%s' is crash-looping.", pod.Name),
		Disease:  "oom-killed",
		Cause:    "The newest pod template's memory limit is too low: the container is OOMKilled on start and crash-loops.",
		Evidence: fmt.Sprintf("lastState.terminated.reason = OOMKilled; memory limit: %s", pod.Spec.Containers[0].Resources.Limits.Memory()),
		Suggest:  rolloutUndo(s),
	}
}
