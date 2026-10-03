package main

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
)

const maxRounds = 5

type doctor struct {
	namespace, app string
	ui             *ui
	cs             kubernetes.Interface
}

type check struct {
	question string
	run      func(s *snapshot) (healthy string, problem *finding)
}

type finding struct {
	symptom  string
	disease  string
	cause    string
	evidence string
	heal     func(*doctor, *snapshot) *treatment
	suggest  string
}

func unknown(symptom, evidence string) *finding {
	return &finding{symptom: symptom, evidence: evidence}
}

func (d *doctor) run(ctx context.Context) (int, error) {
	d.ui.doc("Good day. I'm the chaos doctor. Let's see what the gremlin did.")
	if d.ui.readOnly() {
		d.ui.doc("No terminal attached, so I'll only diagnose and won't change anything.")
	}
	for _, step := range []func(context.Context) error{d.connect, d.chooseNamespace, d.chooseApp} {
		if err := step(ctx); err != nil {
			return 0, err
		}
	}
	d.ui.doc("Examining deployment %s and its service in namespace %s.", d.ui.em(d.app), d.ui.em(d.namespace))

	healed := false
	for round := 1; round <= maxRounds; round++ {
		if round > 1 {
			d.ui.doc("Let me examine the patient again (round %d).", round)
		}
		s, err := d.takeSnapshot(ctx)
		if err != nil {
			return 0, err
		}
		f := d.examine(s)
		d.ui.printf("\n")
		if f == nil && s.requestErr != nil {
			d.ui.warn("Everything I could check looks fine, but I could not verify that requests get through.")
		}
		switch {
		case f == nil && healed:
			d.ui.doc("%sThe patient is healthy again.%s", d.ui.green, d.ui.reset)
			return exitHealed, nil
		case f == nil:
			d.ui.doc("%sClean bill of health.%s Nothing looks wrong with '%s'.", d.ui.green, d.ui.reset, d.app)
			return exitHealthy, nil
		case f.disease == "":
			d.reportUnknown(f)
			return exitUnhealed, nil
		}
		d.ui.printf("Diagnosis [%s]: %s\n", f.disease, f.cause)
		d.ui.evidence(f.evidence)
		if !d.treat(ctx, f, s) {
			if healed {
				d.ui.doc("Leaving this one untreated; the treatments I already applied stay in place.")
			} else {
				d.ui.doc("Leaving it untreated. Nothing was changed.")
			}
			return exitUnhealed, nil
		}
		healed = true
	}
	d.ui.doc("I gave up after %d rounds; the patient keeps changing under me.", maxRounds)
	return exitUnhealed, nil
}

// The first check that finds a problem names the root cause; later checks would only see its symptoms.
func (d *doctor) examine(s *snapshot) *finding {
	for _, c := range checks {
		d.ui.step(c.question)
		healthy, problem := c.run(s)
		if problem != nil {
			d.ui.bad("%s", problem.symptom)
			return problem
		}
		d.ui.ok("%s", healthy)
	}
	return nil
}

func (d *doctor) reportUnknown(f *finding) {
	d.ui.doc("%sI don't know what is wrong.%s It matches none of the diseases I can recognise, and I won't guess.", d.ui.bold, d.ui.reset)
	d.ui.printf("\n  What I observed:\n")
	d.ui.evidence(f.evidence)
	d.ui.printf("\n  What I did not check (a human should look here next):\n")
	for _, next := range []string{
		fmt.Sprintf("app logs:          kubectl -n %s logs deploy/%s --tail=50", d.namespace, d.app),
		fmt.Sprintf("recent events:     kubectl -n %s get events --sort-by=.lastTimestamp", d.namespace),
		fmt.Sprintf("quotas and limits: kubectl -n %s describe resourcequota,limitrange", d.namespace),
		fmt.Sprintf("network policies:  kubectl -n %s get networkpolicy", d.namespace),
		"node health:       kubectl describe nodes | grep -A8 Conditions",
	} {
		d.ui.evidence(next)
	}
	d.ui.printf("Diagnosis [unknown]\n")
}
