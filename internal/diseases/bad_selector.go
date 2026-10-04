package diseases

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func checkBadSelector(s *cluster.Snapshot) (string, *Finding) {
	if s.Service == nil || len(s.Service.Spec.Ports) == 0 {
		return "", unknown(fmt.Sprintf("There is no service '%s' with a port.", s.App), "service "+s.App+" does not exist or exposes no port")
	}
	selector, labels := cluster.LabelString(s.Service.Spec.Selector), cluster.LabelString(s.Deployment.Spec.Template.Labels)
	if selects(s.Service.Spec.Selector, s.Deployment.Spec.Template.Labels) {
		return "Selector " + selector + " matches the pod labels.", nil
	}
	return "", &Finding{
		Symptom:  fmt.Sprintf("Its selector %s does not match the pod labels %s.", selector, labels),
		Disease:  "bad-selector",
		Cause:    fmt.Sprintf("Service '%s' selects %s, but the app's pods are labelled %s, so the service has no endpoints.", s.App, selector, labels),
		Evidence: fmt.Sprintf("service selector %s vs pod template labels %s", selector, labels),
		Heal:     fixSelector,
	}
}

func fixSelector(env Env, s *cluster.Snapshot) *Treatment {
	selector := s.Deployment.Spec.Selector.MatchLabels
	patch, _ := json.Marshal([]map[string]any{{"op": "replace", "path": "/spec/selector", "value": selector}})
	return &Treatment{
		Source:  "the deployment's own selector " + cluster.LabelString(selector),
		Kubectl: fmt.Sprintf("kubectl -n %s patch service %s --type=json -p '%s'", s.Namespace, s.App, patch),
		Apply: func(ctx context.Context) error {
			_, err := env.Client.CoreV1().Services(s.Namespace).Patch(ctx, s.App, types.JSONPatchType, patch, metav1.PatchOptions{})
			return err
		},
	}
}

func selects(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}
