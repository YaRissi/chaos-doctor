package main

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func checkBadSelector(s *snapshot) (string, *finding) {
	if s.service == nil || len(s.service.Spec.Ports) == 0 {
		return "", unknown(fmt.Sprintf("There is no service '%s' with a port.", s.app), "service "+s.app+" does not exist or exposes no port")
	}
	selector, labels := labelString(s.service.Spec.Selector), labelString(s.deploy.Spec.Template.Labels)
	if selects(s.service.Spec.Selector, s.deploy.Spec.Template.Labels) {
		return "Selector " + selector + " matches the pod labels.", nil
	}
	return "", &finding{
		symptom:  fmt.Sprintf("Its selector %s does not match the pod labels %s.", selector, labels),
		disease:  "bad-selector",
		cause:    fmt.Sprintf("Service '%s' selects %s, but the app's pods are labelled %s, so the service has no endpoints.", s.app, selector, labels),
		evidence: fmt.Sprintf("service selector %s vs pod template labels %s", selector, labels),
		heal:     fixSelector,
	}
}

func fixSelector(d *doctor, s *snapshot) *treatment {
	selector := s.deploy.Spec.Selector.MatchLabels
	patch, _ := json.Marshal([]map[string]any{{"op": "replace", "path": "/spec/selector", "value": selector}})
	return &treatment{
		source:  "the deployment's own selector " + labelString(selector),
		kubectl: fmt.Sprintf("kubectl -n %s patch service %s --type=json -p '%s'", s.namespace, s.app, patch),
		apply: func(ctx context.Context) error {
			_, err := d.cs.CoreV1().Services(s.namespace).Patch(ctx, s.app, types.JSONPatchType, patch, metav1.PatchOptions{})
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
