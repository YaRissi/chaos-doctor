package diseases

import (
	"context"
	"fmt"
	"strconv"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const maxReplicas = 99

func checkScaledToZero(s *cluster.Snapshot) (string, *Finding) {
	if replicas := cluster.DesiredReplicas(s.Deployment); replicas > 0 {
		return fmt.Sprintf("It wants %d replicas.", replicas), nil
	}
	return "", &Finding{
		Symptom:  "It wants 0 replicas, so nothing is running.",
		Disease:  "scaled-to-zero",
		Cause:    fmt.Sprintf("Deployment '%s' is scaled to 0 replicas, so no pod exists to serve traffic.", s.App),
		Evidence: ".spec.replicas = 0 (Kubernetes still reports Available=True: zero of zero pods are available)",
		Heal:     scaleUp,
	}
}

func scaleUp(env Env, s *cluster.Snapshot) *Treatment {
	source, fallback := "", ""
	if want := s.Desired.Deployment; want != nil && cluster.DesiredReplicas(want) > 0 {
		fallback = strconv.Itoa(int(cluster.DesiredReplicas(want)))
		source = s.Desired.Source + " wants " + fallback + " replicas"
	}
	replicas, ok := env.UI.Ask(fmt.Sprintf("How many replicas should '%s' run?", s.App), fallback, func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && strconv.Itoa(n) == v && n >= 1 && n <= maxReplicas
	})
	if !ok {
		env.UI.Warn("I don't know how many replicas '%s' should run, and I won't guess.", s.App)
		return nil
	}
	if replicas != fallback {
		source = "you asked for " + replicas
	}
	return &Treatment{
		Source:  source,
		Kubectl: fmt.Sprintf("kubectl -n %s scale deployment/%s --replicas=%s", s.Namespace, s.App, replicas),
		Apply: func(ctx context.Context) error {
			patch := []byte(`{"spec":{"replicas":` + replicas + `}}`)
			_, err := env.Client.AppsV1().Deployments(s.Namespace).Patch(ctx, s.App, types.MergePatchType, patch, metav1.PatchOptions{})
			return err
		},
		WaitRollout: true,
	}
}
