package diseases

import (
	"fmt"
	"strings"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	corev1 "k8s.io/api/core/v1"
)

func readinessFailing(s *cluster.Snapshot, pod *corev1.Pod) *Finding {
	ev := cluster.LatestEvent(s.Events, func(ev corev1.Event) bool {
		return ev.InvolvedObject.Name == pod.Name && ev.Reason == "Unhealthy" && strings.HasPrefix(ev.Message, "Readiness probe failed")
	})
	if ev == nil {
		return nil
	}
	return &Finding{
		Symptom:  fmt.Sprintf("Pod '%s' runs but is not Ready.", pod.Name),
		Disease:  "readiness-failing",
		Cause:    "The newest pods run but fail their readiness probe, so they never receive traffic and the rollout is stuck.",
		Evidence: ev.Reason + ": " + ev.Message,
		Suggest:  rolloutUndo(s),
	}
}
