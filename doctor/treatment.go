package main

import (
	"context"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	rolloutTimeout = 2 * time.Minute
	rolloutPoll    = 2 * time.Second
	endpointSettle = 3 * time.Second
)

type treatment struct {
	source      string
	kubectl     string
	apply       func(context.Context) error
	waitRollout bool
}

func (d *doctor) treat(ctx context.Context, f *finding, s *snapshot) bool {
	if f.heal == nil {
		d.ui.doc("I don't heal this on my own; the right setting is a human decision. Suggested next step: %s", f.suggest)
		return false
	}
	t := f.heal(d, s)
	if t == nil {
		return false
	}
	d.ui.doc("Treatment based on: %s.", t.source)
	d.ui.printf("  %s$ %s%s\n", d.ui.dim, t.kubectl, d.ui.reset)
	if !d.ui.confirm("Shall I heal it? (y/n)") {
		return false
	}
	if err := t.apply(ctx); err != nil {
		d.ui.bad("The treatment failed.")
		d.ui.evidence(err.Error())
		return false
	}
	d.ui.ok("Treatment applied.")
	if t.waitRollout {
		d.waitForRollout(ctx)
	} else {
		time.Sleep(endpointSettle)
	}
	return true
}

func (d *doctor) patchDeployment(patchType types.PatchType, patch string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := d.cs.AppsV1().Deployments(d.namespace).Patch(ctx, d.app, patchType, []byte(patch), metav1.PatchOptions{})
		return err
	}
}

func (d *doctor) waitForRollout(ctx context.Context) {
	d.ui.doc("Waiting for the rollout to finish (up to %s)...", rolloutTimeout)
	for deadline := time.Now().Add(rolloutTimeout); time.Now().Before(deadline); time.Sleep(rolloutPoll) {
		dep, err := d.cs.AppsV1().Deployments(d.namespace).Get(ctx, d.app, metav1.GetOptions{})
		if err == nil && rolledOut(dep) {
			return
		}
	}
	d.ui.warn("The rollout did not finish in time; I'll look again.")
}

func rolledOut(dep *appsv1.Deployment) bool {
	want, st := desiredReplicas(dep), dep.Status
	return st.ObservedGeneration >= dep.Generation && st.UpdatedReplicas == want && st.AvailableReplicas == want && st.Replicas == want
}
