package diseases

import (
	"fmt"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

func unschedulable(s *cluster.Snapshot, pod *corev1.Pod) *Finding {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Reason == corev1.PodReasonUnschedulable {
			return &Finding{
				Symptom:  fmt.Sprintf("Pod '%s' is Pending.", pod.Name),
				Disease:  "unschedulable",
				Cause:    "No node can take the newest pods, so they stay Pending forever.",
				Evidence: "PodScheduled=False: " + c.Message,
				Suggest:  rolloutUndo(s),
			}
		}
	}
	return nil
}
