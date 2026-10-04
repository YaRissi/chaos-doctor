package doctor

import (
	"context"
	"time"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	"github.com/YaRissi/chaos-doctor/internal/diseases"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	rolloutTimeout = 2 * time.Minute
	rolloutPoll    = 2 * time.Second
	endpointSettle = 3 * time.Second
)

func (d *Doctor) treat(ctx context.Context, f *diseases.Finding, s *cluster.Snapshot) bool {
	if f.Heal == nil {
		d.UI.Doc("I don't heal this on my own; the right setting is a human decision. Suggested next step: %s", f.Suggest)
		return false
	}
	t := f.Heal(diseases.Env{UI: d.UI, Client: d.client}, s)
	if t == nil {
		return false
	}
	d.UI.Doc("Treatment based on: %s.", t.Source)
	d.UI.Printf("  %s$ %s%s\n", d.UI.Dim, t.Kubectl, d.UI.Reset)
	if !d.UI.Confirm("Shall I heal it? (y/n)") {
		return false
	}
	if err := t.Apply(ctx); err != nil {
		d.UI.Bad("The treatment failed.")
		d.UI.Evidence(err.Error())
		return false
	}
	d.UI.OK("Treatment applied.")
	if t.WaitRollout {
		d.waitForRollout(ctx)
	} else {
		time.Sleep(endpointSettle)
	}
	return true
}

func (d *Doctor) waitForRollout(ctx context.Context) {
	d.UI.Doc("Waiting for the rollout to finish (up to %s)...", rolloutTimeout)
	for deadline := time.Now().Add(rolloutTimeout); time.Now().Before(deadline); time.Sleep(rolloutPoll) {
		dep, err := d.client.AppsV1().Deployments(d.Namespace).Get(ctx, d.App, metav1.GetOptions{})
		if err == nil && rolledOut(dep) {
			return
		}
	}
	d.UI.Warn("The rollout did not finish in time; I'll look again.")
}

func rolledOut(dep *appsv1.Deployment) bool {
	want, st := cluster.DesiredReplicas(dep), dep.Status
	return st.ObservedGeneration >= dep.Generation && st.UpdatedReplicas == want && st.AvailableReplicas == want && st.Replicas == want
}
