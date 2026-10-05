package diseases

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/YaRissi/chaos-doctor/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Order is the dependency chain: a later check may rely on an earlier one having passed.
var Checks = []Check{
	{"Does every ConfigMap the pods need still exist?", checkMissingConfig},
	{"Is the deployment asking for any pods?", checkScaledToZero},
	{"How are the pods of the newest rollout doing?", checkRollout},
	{"Does the service select the app's pods?", checkBadSelector},
	{"Does the service have ready endpoints?", checkEndpoints},
	{"Does a request through the service get an answer?", checkRequest},
}

// Why a pod of the newest rollout is not Ready; asked in order for the first such pod.
var podDiseases = []func(*cluster.Snapshot, *corev1.Pod) *Finding{
	badImage,
	oomKilled,
	unschedulable,
	readinessFailing,
}

func checkRollout(s *cluster.Snapshot) (string, *Finding) {
	rs := s.NewestReplicaSet()
	if rs == nil {
		return "", unknown("I can't find the deployment's current ReplicaSet.", "no ReplicaSet matches the deployment's current revision")
	}
	var pods []corev1.Pod
	for _, p := range s.Pods {
		if slices.ContainsFunc(p.OwnerReferences, func(ref metav1.OwnerReference) bool { return ref.UID == rs.UID }) {
			pods = append(pods, p)
		}
	}
	want := int32(1)
	if rs.Spec.Replicas != nil {
		want = *rs.Spec.Replicas
	}
	if len(pods) == 0 || int32(len(pods)) < want {
		return "", unknown(fmt.Sprintf("ReplicaSet '%s' has %d of %d pods.", rs.Name, len(pods), want),
			fmt.Sprintf("ReplicaSet %s has %d of %d pods%s", rs.Name, len(pods), want, cluster.LatestWarning(s.Events, rs.Name)))
	}
	for i := range pods {
		if cluster.PodReady(&pods[i]) {
			continue
		}
		for _, disease := range podDiseases {
			if f := disease(s, &pods[i]); f != nil {
				return "", f
			}
		}
		return "", unknown(fmt.Sprintf("Pod '%s' is not Ready.", pods[i].Name),
			fmt.Sprintf("pod %s is not Ready (phase %s)%s", pods[i].Name, pods[i].Status.Phase, cluster.LatestWarning(s.Events, pods[i].Name)))
	}
	return fmt.Sprintf("All %d pods of '%s' are Ready.", len(pods), rs.Name), nil
}

func checkEndpoints(s *cluster.Snapshot) (string, *Finding) {
	ready := 0
	for _, slice := range s.EndpointSlices {
		for _, ep := range slice.Endpoints {
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				ready++
			}
		}
	}
	if ready == 0 {
		return "", unknown("No ready endpoints.", "EndpointSlices for "+s.App+" list no ready endpoint")
	}
	return fmt.Sprintf("%d ready endpoint(s).", ready), nil
}

func checkRequest(s *cluster.Snapshot) (string, *Finding) {
	err := s.RequestErr
	switch {
	case err == nil:
		return fmt.Sprintf("GET / through service '%s' answered successfully.", s.App), nil
	case RequestUntestable(err):
		return "I could not test it, so I'm not counting it against the patient: " + err.Error(), nil
	}
	return "", unknown("The request failed.", "request through the service failed: "+err.Error())
}

func RequestUntestable(err error) bool {
	return apierrors.IsForbidden(err) || apierrors.IsTimeout(err) || errors.Is(err, context.DeadlineExceeded)
}
