package main

import (
	"context"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func checkMissingConfig(s *snapshot) (string, *finding) {
	refs := configMapRefs(&s.deploy.Spec.Template.Spec)
	for _, name := range refs {
		if s.configMaps[name] {
			continue
		}
		evidence := fmt.Sprintf("the pod template mounts configMap %s, and the API reports it as NotFound", name)
		if ev := latestEvent(s.events, func(ev corev1.Event) bool {
			return ev.Reason == "FailedMount" && strings.Contains(ev.Message, `"`+name+`"`)
		}); ev != nil {
			evidence = ev.Message
		}
		cause := fmt.Sprintf("ConfigMap '%s' was deleted, so new pods cannot mount it and never start.", name)
		if len(s.pods) > 0 && allRunning(s.pods) {
			cause = fmt.Sprintf("ConfigMap '%s' was deleted; running pods still serve their old copy, but every new or restarted pod will hang in ContainerCreating.", name)
		}
		return "", &finding{
			symptom:  fmt.Sprintf("ConfigMap '%s' is referenced by the pod template but does not exist.", name),
			disease:  "missing-config",
			cause:    cause,
			evidence: evidence,
			heal:     func(d *doctor, s *snapshot) *treatment { return restoreConfigMap(d, s, name) },
		}
	}
	if len(refs) == 0 {
		return "The pods don't reference any ConfigMap.", nil
	}
	return "All referenced ConfigMaps exist: " + strings.Join(refs, ", ") + ".", nil
}

func restoreConfigMap(d *doctor, s *snapshot, name string) *treatment {
	original, ok := s.desired.configMaps[name]
	if !ok {
		d.ui.warn("I don't know what was inside '%s' and I won't invent it. Re-deploy it from its source (e.g. 'helm upgrade').", name)
		return nil
	}
	cm := original.DeepCopy()
	cm.Namespace = s.namespace
	return &treatment{
		source:  s.desired.source + ", which still contains ConfigMap '" + name + "'",
		kubectl: fmt.Sprintf(`helm get manifest %s -n %s | yq 'select(.kind == "ConfigMap")' | kubectl -n %s create -f -`, s.desired.release, s.namespace, s.namespace),
		apply: func(ctx context.Context) error {
			_, err := d.cs.CoreV1().ConfigMaps(s.namespace).Create(ctx, cm, metav1.CreateOptions{})
			return err
		},
		waitRollout: true,
	}
}

func configMapRefs(spec *corev1.PodSpec) []string {
	var refs []string
	add := func(name string, optional *bool) {
		if (optional == nil || !*optional) && !slices.Contains(refs, name) {
			refs = append(refs, name)
		}
	}
	for _, v := range spec.Volumes {
		if v.ConfigMap != nil {
			add(v.ConfigMap.Name, v.ConfigMap.Optional)
		}
	}
	for _, c := range spec.Containers {
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				add(ef.ConfigMapRef.Name, ef.ConfigMapRef.Optional)
			}
		}
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				add(env.ValueFrom.ConfigMapKeyRef.Name, env.ValueFrom.ConfigMapKeyRef.Optional)
			}
		}
	}
	return refs
}

func allRunning(pods []corev1.Pod) bool {
	for _, p := range pods {
		if p.Status.Phase != corev1.PodRunning {
			return false
		}
	}
	return true
}
