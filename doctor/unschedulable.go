package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

func unschedulable(s *snapshot, pod *corev1.Pod) *finding {
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Reason == corev1.PodReasonUnschedulable {
			return &finding{
				symptom:  fmt.Sprintf("Pod '%s' is Pending.", pod.Name),
				disease:  "unschedulable",
				cause:    "No node can take the newest pods, so they stay Pending forever.",
				evidence: "PodScheduled=False: " + c.Message,
				suggest:  rolloutUndo(s),
			}
		}
	}
	return nil
}
