// Package diseases holds what the doctor can recognise: one file per disease, ordered in checks.go.
package diseases

import (
	"context"
	"fmt"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	"github.com/YaRissi/chaos-doctor/internal/ui"
	"k8s.io/client-go/kubernetes"
)

type Check struct {
	Question string
	Run      func(s *cluster.Snapshot) (healthy string, problem *Finding)
}

type Finding struct {
	Symptom  string
	Disease  string
	Cause    string
	Evidence string
	Heal     func(Env, *cluster.Snapshot) *Treatment
	Suggest  string
}

type Env struct {
	UI     *ui.UI
	Client kubernetes.Interface
}

type Treatment struct {
	Source      string
	Kubectl     string
	Apply       func(context.Context) error
	WaitRollout bool
}

func unknown(symptom, evidence string) *Finding {
	return &Finding{Symptom: symptom, Evidence: evidence}
}

func rolloutUndo(s *cluster.Snapshot) string {
	return fmt.Sprintf("kubectl -n %s rollout undo deployment/%s", s.Namespace, s.App)
}
