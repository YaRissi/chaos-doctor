package diseases

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func badImage(_ *cluster.Snapshot, pod *corev1.Pod) *Finding {
	waiting := cluster.WaitingState(pod)
	if waiting == nil || !slices.Contains([]string{"ErrImagePull", "ImagePullBackOff", "InvalidImageName"}, waiting.Reason) {
		return nil
	}
	image := pod.Spec.Containers[0].Image
	symptom := fmt.Sprintf("Pod '%s' cannot pull image '%s'.", pod.Name, image)
	if waiting.Reason != "InvalidImageName" && !strings.Contains(waiting.Message, "not found") && !strings.Contains(waiting.Message, "NotFound") {
		return unknown(symptom, "the registry did not say 'not found' (auth, network or rate limit?): "+waiting.Message)
	}
	return &Finding{
		Symptom:  symptom,
		Disease:  "bad-image",
		Cause:    fmt.Sprintf("The rollout uses image '%s', which does not exist in the registry, so new pods can't start.", image),
		Evidence: waiting.Reason + ": " + waiting.Message,
		Heal:     fixImage,
	}
}

func fixImage(env Env, s *cluster.Snapshot) *Treatment {
	container := s.Deployment.Spec.Template.Spec.Containers[0].Name
	source, fallback := "", ""
	if want := s.Desired.Deployment; want != nil {
		fallback, source = imageOf(want.Spec.Template.Spec.Containers, container), s.Desired.Source
	}
	if prev := s.PreviousHealthyReplicaSet(); fallback == "" && prev != nil {
		fallback, source = imageOf(prev.Spec.Template.Spec.Containers, container), "the last healthy ReplicaSet "+prev.Name
	}
	image, ok := env.UI.Ask(fmt.Sprintf("Which image should container '%s' run?", container), fallback, func(v string) bool {
		return v != "" && !strings.ContainsAny(v, " \t'\"")
	})
	if !ok {
		env.UI.Warn("I don't know which image is correct, and I won't guess.")
		return nil
	}
	if image != fallback {
		source = "you named " + image
	}
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
		"containers": []map[string]string{{"name": container, "image": image}},
	}}}})
	return &Treatment{
		Source:  source + " → " + image,
		Kubectl: fmt.Sprintf("kubectl -n %s set image deployment/%s %s=%s", s.Namespace, s.App, container, image),
		Apply: func(ctx context.Context) error {
			_, err := env.Client.AppsV1().Deployments(s.Namespace).Patch(ctx, s.App, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
			return err
		},
		WaitRollout: true,
	}
}

func imageOf(containers []corev1.Container, name string) string {
	for _, c := range containers {
		if c.Name == name {
			return c.Image
		}
	}
	return ""
}
