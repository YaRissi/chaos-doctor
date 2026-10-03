package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Order is the dependency chain: a later check may rely on an earlier one having passed
// (bad-target-port assumes bad-selector already confirmed the service exists).
var checks = []check{
	{"Does every ConfigMap the pods need still exist?", checkMissingConfig},
	{"Is the deployment asking for any pods?", checkScaledToZero},
	{"How are the pods of the newest rollout doing?", checkRollout},
	{"Does the service select the app's pods?", checkBadSelector},
	{"Does the service send traffic to a port the container declares?", checkBadTargetPort},
	{"Does the service have ready endpoints?", checkEndpoints},
	{"Does a request through the service get an answer?", checkRequest},
}

// Why a pod of the newest rollout is not Ready; asked in order for the first such pod.
var podDiseases = []func(*snapshot, *corev1.Pod) *finding{
	badImage,
	oomKilled,
	unschedulable,
	readinessFailing,
}

func checkRollout(s *snapshot) (string, *finding) {
	rs := newestReplicaSet(s)
	if rs == nil {
		return "", unknown("I can't find the deployment's current ReplicaSet.", "no ReplicaSet matches the deployment's current revision")
	}
	var pods []corev1.Pod
	for _, p := range s.pods {
		if slices.ContainsFunc(p.OwnerReferences, func(ref metav1.OwnerReference) bool { return ref.UID == rs.UID }) {
			pods = append(pods, p)
		}
	}
	if len(pods) == 0 {
		return "", unknown(fmt.Sprintf("ReplicaSet '%s' has no pods at all.", rs.Name), "ReplicaSet "+rs.Name+" has 0 pods"+latestWarning(s.events, rs.Name))
	}
	for i := range pods {
		if podReady(&pods[i]) {
			continue
		}
		for _, disease := range podDiseases {
			if f := disease(s, &pods[i]); f != nil {
				return "", f
			}
		}
		return "", unknown(fmt.Sprintf("Pod '%s' is not Ready.", pods[i].Name),
			fmt.Sprintf("pod %s is not Ready (phase %s)%s", pods[i].Name, pods[i].Status.Phase, latestWarning(s.events, pods[i].Name)))
	}
	return fmt.Sprintf("All %d pods of '%s' are Ready.", len(pods), rs.Name), nil
}

func checkEndpoints(s *snapshot) (string, *finding) {
	ready := 0
	for _, slice := range s.slices {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				ready++
			}
		}
	}
	if ready == 0 {
		return "", unknown("No ready endpoints.", "EndpointSlices for "+s.app+" list no ready endpoint")
	}
	return fmt.Sprintf("%d ready endpoint(s).", ready), nil
}

func checkRequest(s *snapshot) (string, *finding) {
	err := s.requestErr
	switch {
	case err == nil:
		return fmt.Sprintf("GET / through service '%s' answered successfully.", s.app), nil
	case apierrors.IsForbidden(err) || apierrors.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded):
		return "I could not test it, so I'm not counting it against the patient: " + err.Error(), nil
	}
	return "", unknown("The request failed.", "request through the service failed: "+err.Error())
}
