package main

import (
	"fmt"
	"strconv"

	"k8s.io/apimachinery/pkg/types"
)

const maxReplicas = 99

func checkScaledToZero(s *snapshot) (string, *finding) {
	if replicas := desiredReplicas(s.deploy); replicas > 0 {
		return fmt.Sprintf("It wants %d replicas.", replicas), nil
	}
	return "", &finding{
		symptom:  "It wants 0 replicas, so nothing is running.",
		disease:  "scaled-to-zero",
		cause:    fmt.Sprintf("Deployment '%s' is scaled to 0 replicas, so no pod exists to serve traffic.", s.app),
		evidence: ".spec.replicas = 0 (Kubernetes still reports Available=True: zero of zero pods are available)",
		heal:     scaleUp,
	}
}

func scaleUp(d *doctor, s *snapshot) *treatment {
	source, fallback := "", ""
	if want := s.desired.deployment; want != nil && want.Spec.Replicas != nil && *want.Spec.Replicas > 0 {
		fallback = strconv.Itoa(int(*want.Spec.Replicas))
		source = s.desired.source + " wants " + fallback + " replicas"
	}
	replicas, ok := d.ui.ask(fmt.Sprintf("How many replicas should '%s' run?", s.app), fallback, func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= maxReplicas
	})
	if !ok {
		d.ui.warn("I don't know how many replicas '%s' should run, and I won't guess.", s.app)
		return nil
	}
	if replicas != fallback {
		source = "you asked for " + replicas
	}
	return &treatment{
		source:      source,
		kubectl:     fmt.Sprintf("kubectl -n %s scale deployment/%s --replicas=%s", s.namespace, s.app, replicas),
		apply:       d.patchDeployment(types.MergePatchType, `{"spec":{"replicas":`+replicas+`}}`),
		waitRollout: true,
	}
}
