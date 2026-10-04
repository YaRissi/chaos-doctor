// Package doctor runs the examination: take a snapshot, run the checks, treat the first finding, repeat.
package doctor

import (
	"context"
	"fmt"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	"github.com/YaRissi/chaos-doctor/internal/diseases"
	"github.com/YaRissi/chaos-doctor/internal/ui"
	"k8s.io/client-go/kubernetes"
)

const maxRounds = 5

type Doctor struct {
	Namespace, App string
	UI             *ui.UI
	client         kubernetes.Interface
}

func (d *Doctor) Run(ctx context.Context) (bool, error) {
	d.UI.Doc("Good day. I'm the chaos doctor. Let's see what the gremlin did.")
	if d.UI.ReadOnly() {
		d.UI.Doc("No terminal attached, so I'll only diagnose and won't change anything.")
	}
	for _, step := range []func(context.Context) error{d.connect, d.chooseNamespace, d.chooseApp} {
		if err := step(ctx); err != nil {
			return false, err
		}
	}
	d.UI.Doc("Examining deployment %s and its service in namespace %s.", d.UI.Em(d.App), d.UI.Em(d.Namespace))

	healed := false
	for round := 1; round <= maxRounds; round++ {
		if round > 1 {
			d.UI.Doc("Let me examine the patient again (round %d).", round)
		}
		s, err := cluster.Take(ctx, d.client, d.Namespace, d.App)
		if err != nil {
			return false, fmt.Errorf("reading deployment %s: %w", d.App, err)
		}
		f := d.examine(s)
		d.UI.Printf("\n")
		if f == nil && s.RequestErr != nil {
			d.UI.Warn("Everything I could check looks fine, but I could not verify that requests get through.")
		}
		switch {
		case f == nil && healed:
			d.UI.Doc("%sThe patient is healthy again.%s", d.UI.Green, d.UI.Reset)
			return true, nil
		case f == nil:
			d.UI.Doc("%sClean bill of health.%s Nothing looks wrong with '%s'.", d.UI.Green, d.UI.Reset, d.App)
			return true, nil
		case f.Disease == "":
			d.reportUnknown(f)
			return false, nil
		}
		d.UI.Printf("Diagnosis [%s]: %s\n", f.Disease, f.Cause)
		d.UI.Evidence(f.Evidence)
		if !d.treat(ctx, f, s) {
			if healed {
				d.UI.Doc("Leaving this one untreated; the treatments I already applied stay in place.")
			} else {
				d.UI.Doc("Leaving it untreated. Nothing was changed.")
			}
			return false, nil
		}
		healed = true
	}
	d.UI.Doc("I gave up after %d rounds; the patient keeps changing under me.", maxRounds)
	return false, nil
}

// The first check that finds a problem names the root cause; later checks would only see its symptoms.
func (d *Doctor) examine(s *cluster.Snapshot) *diseases.Finding {
	for _, c := range diseases.Checks {
		d.UI.Step(c.Question)
		healthy, problem := c.Run(s)
		if problem != nil {
			d.UI.Bad("%s", problem.Symptom)
			return problem
		}
		d.UI.OK("%s", healthy)
	}
	return nil
}

func (d *Doctor) reportUnknown(f *diseases.Finding) {
	d.UI.Doc("%sI don't know what is wrong.%s It matches none of the diseases I can recognise, and I won't guess.", d.UI.Bold, d.UI.Reset)
	d.UI.Printf("\n  What I observed:\n")
	d.UI.Evidence(f.Evidence)
	d.UI.Printf("\n  What I did not check (a human should look here next):\n")
	for _, next := range []string{
		fmt.Sprintf("app logs:          kubectl -n %s logs deploy/%s --tail=50", d.Namespace, d.App),
		fmt.Sprintf("recent events:     kubectl -n %s get events --sort-by=.lastTimestamp", d.Namespace),
		fmt.Sprintf("quotas and limits: kubectl -n %s describe resourcequota,limitrange", d.Namespace),
		fmt.Sprintf("network policies:  kubectl -n %s get networkpolicy", d.Namespace),
		"node health:       kubectl describe nodes | grep -A8 Conditions",
	} {
		d.UI.Evidence(next)
	}
	d.UI.Printf("Diagnosis [unknown]\n")
}
