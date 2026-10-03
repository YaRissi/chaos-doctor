package main

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func checkBadTargetPort(s *snapshot) (string, *finding) {
	port := s.service.Spec.Ports[0]
	target := port.TargetPort
	if target.Type == intstr.Int && target.IntVal == 0 {
		target = intstr.FromInt32(port.Port)
	}
	declared := containerPorts(&s.deploy.Spec.Template.Spec)
	if slices.Contains(declared, target.String()) {
		return fmt.Sprintf("It forwards to port '%s', which the container declares.", target.String()), nil
	}
	return "", &finding{
		symptom:  fmt.Sprintf("It forwards to port '%s', but the container only declares: %s.", target.String(), strings.Join(declared, " ")),
		disease:  "bad-target-port",
		cause:    fmt.Sprintf("Service '%s' sends traffic to port '%s', which no container declares, so requests most likely hit a closed port.", s.app, target.String()),
		evidence: fmt.Sprintf("targetPort %s vs container ports: %s", target.String(), strings.Join(declared, " ")),
		suggest:  fmt.Sprintf("kubectl -n %s edit service %s", s.namespace, s.app),
	}
}

func containerPorts(spec *corev1.PodSpec) []string {
	var ports []string
	for _, c := range spec.Containers {
		for _, p := range c.Ports {
			if p.Name != "" {
				ports = append(ports, p.Name)
			}
			ports = append(ports, strconv.Itoa(int(p.ContainerPort)))
		}
	}
	return ports
}
