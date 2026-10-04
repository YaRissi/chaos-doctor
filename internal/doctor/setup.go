package doctor

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/YaRissi/chaos-doctor/internal/ui"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const requestTimeout = 5 * time.Second

var NameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

func (d *Doctor) connect(ctx context.Context) error {
	d.UI.Step("Can I reach the cluster?")
	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(), &clientcmd.ConfigOverrides{})
	cfg, err := loader.ClientConfig()
	if err != nil {
		return fmt.Errorf("loading kubeconfig (for the demo: 'just up'): %w", err)
	}
	cfg.Timeout = requestTimeout
	if d.client, err = kubernetes.NewForConfig(cfg); err != nil {
		return err
	}
	if _, err := d.client.Discovery().RESTClient().Get().AbsPath("/readyz").DoRaw(ctx); err != nil {
		return fmt.Errorf("the cluster at %s is not answering, is it running? (for the demo: 'just up'): %w", cfg.Host, err)
	}
	d.UI.OK("The API server at %s is ready.", cfg.Host)
	return nil
}

func (d *Doctor) chooseNamespace(ctx context.Context) error {
	if d.Namespace == "" {
		if !d.UI.Interactive {
			return fmt.Errorf("no terminal to ask for a namespace; pass -n")
		}
		deps, err := d.client.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
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
		d.UI.Doc("Namespaces with deployments: %s", strings.Join(namespaces, " "))
		fallback := ""
		if len(namespaces) == 1 {
			fallback = namespaces[0]
		}
		ns, ok := d.UI.Ask("Which namespace?", fallback, NameRE.MatchString)
		if !ok {
			return fmt.Errorf("no valid namespace after %d tries", ui.MaxTries)
		}
		d.Namespace = ns
	}
	_, err := d.client.CoreV1().Namespaces().Get(ctx, d.Namespace, metav1.GetOptions{})
	return err
}

func (d *Doctor) chooseApp(ctx context.Context) error {
	deps, err := d.client.AppsV1().Deployments(d.Namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("listing deployments: %w", err)
	}
	var apps []string
	for _, dep := range deps.Items {
		apps = append(apps, dep.Name)
	}
	switch {
	case d.App != "" && slices.Contains(apps, d.App):
		return nil
	case d.App != "" || len(apps) == 0:
		return fmt.Errorf("there is no deployment %q in %q, I can see: %s", d.App, d.Namespace, strings.Join(apps, " "))
	case len(apps) == 1:
		d.App = apps[0]
		d.UI.Doc("Only one patient here: %s.", d.UI.Em(d.App))
		return nil
	case !d.UI.Interactive:
		return fmt.Errorf("no terminal to ask which deployment; pass -a")
	}
	for i, app := range apps {
		d.UI.Printf("    %d) %s\n", i+1, app)
	}
	choice, ok := d.UI.Ask("Which one? (number)", "", func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && n >= 1 && n <= len(apps)
	})
	if !ok {
		return fmt.Errorf("no valid choice after %d tries", ui.MaxTries)
	}
	n, _ := strconv.Atoi(choice)
	d.App = apps[n-1]
	return nil
}
