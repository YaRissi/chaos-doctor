package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

const (
	exitHealthy      = 0
	exitError        = 1
	exitUnhealed     = 2
	exitHealed       = 3
	exitUnexaminable = 4
	exitInterrupted  = 130

	requestTimeout = 5 * time.Second
)

var version = "dev"

var nameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

type stopError struct{ code int }

func (e stopError) Error() string { return fmt.Sprintf("stopped with exit code %d", e.code) }

func main() { os.Exit(run()) }

func run() int {
	d := &doctor{}
	auto := false
	code := exitHealthy
	cmd := &cobra.Command{
		Use:     "doctor",
		Version: version,
		Short:   "Examines a Deployment and its Service, explains what is wrong and offers to heal it",
		Long: `Examines a Deployment and its Service, explains what is wrong and offers to heal it.

The service is expected to have the same name as the deployment.
Without a terminal and without --auto the doctor only diagnoses, it never changes anything.
Exit codes: 0 healthy, 2 not healed / unknown, 3 healed, 4 could not examine, 1 usage error, 130 interrupted.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, name := range []string{d.namespace, d.app} {
				if name != "" && !nameRE.MatchString(name) {
					return fmt.Errorf("%q is not a valid Kubernetes name", name)
				}
			}
			d.ui = newUI(auto)
			exitOnInterrupt(d.ui)
			var err error
			code, err = d.run(cmd.Context())
			return err
		},
	}
	cmd.Flags().StringVarP(&d.namespace, "namespace", "n", "", "namespace to examine (prompted if omitted)")
	cmd.Flags().StringVarP(&d.app, "app", "a", "", "deployment to examine (discovered if omitted)")
	cmd.Flags().BoolVar(&auto, "auto", false, "heal without asking; refuse whenever the correct value is unknown")

	err := cmd.ExecuteContext(context.Background())
	var stop stopError
	switch {
	case errors.As(err, &stop):
		return stop.code
	case err != nil:
		fmt.Fprintln(os.Stderr, "error:", err)
		return exitError
	}
	return code
}

func exitOnInterrupt(u *ui) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		u.printf("\n")
		u.doc("Interrupted. Nothing is half-applied: every treatment is a single API call. Goodbye.")
		os.Exit(exitInterrupted)
	}()
}
