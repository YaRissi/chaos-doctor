package main

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func (d *doctor) connect(ctx context.Context) error {
	d.ui.step("Can I reach the cluster?")
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	cfg, err := loader.ClientConfig()
	if err != nil {
		d.ui.bad("I can't load a kubeconfig: %v", err)
		d.ui.doc("Point kubectl at a cluster first (for the demo: 'just up').")
		return stopError{exitUnexaminable}
	}
	cfg.Timeout = requestTimeout
	if d.cs, err = kubernetes.NewForConfig(cfg); err != nil {
		return err
	}
	if _, err := d.cs.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx); err != nil {
		d.ui.bad("The cluster at %s is not answering.", cfg.Host)
		d.ui.evidence(err.Error())
		d.ui.doc("Is the cluster running? For the demo: 'just up' (or 'kind get clusters').")
		return stopError{exitUnexaminable}
	}
	d.ui.ok("The API server at %s is ready.", cfg.Host)
	return nil
}

func (d *doctor) chooseNamespace(ctx context.Context) error {
	if d.namespace == "" {
		if !d.ui.interactive {
			return fmt.Errorf("no terminal to ask for a namespace; pass -n")
		}
		deps, err := d.cs.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("listing deployments: %w", err)
		}
		var namespaces []string
		for _, dep := range deps.Items {
			if !strings.HasPrefix(dep.Namespace, "kube-") && !slices.Contains(namespaces, dep.Namespace) {
				namespaces = append(namespaces, dep.Namespace)
			}
		}
		slices.Sort(namespaces)
		d.ui.doc("Namespaces with deployments: %s", strings.Join(namespaces, " "))
		fallback := ""
		if len(namespaces) == 1 {
			fallback = namespaces[0]
		}
		ns, ok := d.ui.ask("Which namespace?", fallback, nameRE.MatchString)
		if !ok {
			return fmt.Errorf("no valid namespace after %d tries", maxTries)
		}
		d.namespace = ns
	}
	if _, err := d.cs.CoreV1().Namespaces().Get(ctx, d.namespace, metav1.GetOptions{}); err != nil {
		d.ui.bad("I can't examine namespace '%s': %v", d.namespace, err)
		return stopError{exitUnexaminable}
	}
	return nil
}

func (d *doctor) chooseApp(ctx context.Context) error {
	deps, err := d.cs.AppsV1().Deployments(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	var apps []string
	for _, dep := range deps.Items {
		apps = append(apps, dep.Name)
	}
	switch {
	case d.app != "" && slices.Contains(apps, d.app):
		return nil
	case d.app != "" || len(apps) == 0:
		d.ui.bad("There is no deployment '%s' in '%s'. I can see: %s", d.app, d.namespace, strings.Join(apps, " "))
		return stopError{exitUnexaminable}
	case len(apps) == 1:
		d.app = apps[0]
		d.ui.doc("Only one patient here: %s.", d.ui.em(d.app))
		return nil
	case !d.ui.interactive:
		return fmt.Errorf("no terminal to ask which deployment; pass -a")
	}
	for i, app := range apps {
		d.ui.printf("    %d) %s\n", i+1, app)
	}
	choice, ok := d.ui.ask("Which one? (number)", "", func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= len(apps)
	})
	if !ok {
		return fmt.Errorf("no valid choice after %d tries", maxTries)
	}
	n, _ := strconv.Atoi(choice)
	d.app = apps[n-1]
	return nil
}
