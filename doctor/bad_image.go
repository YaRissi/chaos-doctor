package main

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func badImage(_ *snapshot, pod *corev1.Pod) *finding {
	waiting := waitingState(pod)
	if waiting == nil || !slices.Contains([]string{"ErrImagePull", "ImagePullBackOff", "InvalidImageName"}, waiting.Reason) {
		return nil
	}
	image := pod.Spec.Containers[0].Image
	symptom := fmt.Sprintf("Pod '%s' cannot pull image '%s'.", pod.Name, image)
	if waiting.Reason != "InvalidImageName" && !strings.Contains(waiting.Message, "not found") && !strings.Contains(waiting.Message, "NotFound") {
		return unknown(symptom, "the registry did not say 'not found' (auth, network or rate limit?): "+waiting.Message)
	}
	return &finding{
		symptom:  symptom,
		disease:  "bad-image",
		cause:    fmt.Sprintf("The rollout uses image '%s', which does not exist in the registry, so new pods can't start.", image),
		evidence: waiting.Reason + ": " + waiting.Message,
		heal:     fixImage,
	}
}

func fixImage(d *doctor, s *snapshot) *treatment {
	container := s.deploy.Spec.Template.Spec.Containers[0].Name
	source, fallback := "", ""
	if want := s.desired.deployment; want != nil {
		fallback, source = imageOf(want.Spec.Template.Spec.Containers, container), s.desired.source
	}
	if prev := previousHealthyReplicaSet(s); fallback == "" && prev != nil {
		fallback, source = imageOf(prev.Spec.Template.Spec.Containers, container), "the last healthy ReplicaSet "+prev.Name
	}
	image, ok := d.ui.ask(fmt.Sprintf("Which image should container '%s' run?", container), fallback, func(v string) bool {
		return v != "" && !strings.ContainsAny(v, " \t'\"")
	})
	if !ok {
		d.ui.warn("I don't know which image is correct, and I won't guess.")
		return nil
	}
	if image != fallback {
		source = "you named " + image
	}
	patch, _ := json.Marshal(map[string]any{"spec": map[string]any{"template": map[string]any{"spec": map[string]any{
		"containers": []map[string]string{{"name": container, "image": image}},
	}}}})
	return &treatment{
		source:      source + " → " + image,
		kubectl:     fmt.Sprintf("kubectl -n %s set image deployment/%s %s=%s", s.namespace, s.app, container, image),
		apply:       d.patchDeployment(types.StrategicMergePatchType, string(patch)),
		waitRollout: true,
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

func previousHealthyReplicaSet(s *snapshot) *appsv1.ReplicaSet {
	newest := newestReplicaSet(s)
	var best *appsv1.ReplicaSet
	bestRevision := -1
	for i := range s.replicaSets {
		rs := &s.replicaSets[i]
		revision, err := strconv.Atoi(rs.Annotations[revisionAnnotation])
		if err == nil && rs != newest && rs.Status.ReadyReplicas > 0 && revision > bestRevision {
			best, bestRevision = rs, revision
		}
	}
	return best
}
