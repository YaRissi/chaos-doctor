package main

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func readinessFailing(s *snapshot, pod *corev1.Pod) *finding {
	ev := latestEvent(s.events, func(ev corev1.Event) bool {
		return ev.InvolvedObject.Name == pod.Name && ev.Reason == "Unhealthy" && strings.HasPrefix(ev.Message, "Readiness probe failed")
	})
	if ev == nil {
		return nil
	}
	return &finding{
		symptom:  fmt.Sprintf("Pod '%s' runs but is not Ready.", pod.Name),
		disease:  "readiness-failing",
		cause:    "The newest pods run but fail their readiness probe, so they never receive traffic and the rollout is stuck.",
		evidence: ev.Reason + ": " + ev.Message,
		suggest:  rolloutUndo(s),
	}
}
