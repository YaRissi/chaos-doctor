package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/YaRissi/chaos-doctor/internal/doctor"
	"github.com/YaRissi/chaos-doctor/internal/ui"
	"github.com/spf13/cobra"
)

var version = ""

func main() { os.Exit(run()) }

func run() int {
	d := &doctor.Doctor{}
	auto := false
	code := doctor.ExitHealthy
	cmd := &cobra.Command{
		Use:     "doctor",
		Version: buildVersion(),
		Short:   "Examines a Deployment and its Service, explains what is wrong and offers to heal it",
		Long: `Examines a Deployment and its Service, explains what is wrong and offers to heal it.

The service is expected to have the same name as the deployment.
Without a terminal and without --auto the doctor only diagnoses, it never changes anything.
Exit codes: 0 healthy, 2 not healed / unknown, 3 healed, 1 error, 130 interrupted.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, name := range []string{d.Namespace, d.App} {
				if name != "" && !doctor.NameRE.MatchString(name) {
					return fmt.Errorf("%q is not a valid Kubernetes name", name)
				}
			}
			d.UI = ui.New(auto)
			exitOnInterrupt(d.UI)
			var err error
			code, err = d.Run(cmd.Context())
			return err
		},
	}
	cmd.Flags().StringVarP(&d.Namespace, "namespace", "n", "", "namespace to examine (prompted if omitted)")
	cmd.Flags().StringVarP(&d.App, "app", "a", "", "deployment to examine (discovered if omitted)")
	cmd.Flags().BoolVar(&auto, "auto", false, "heal without asking; refuse whenever the correct value is unknown")

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return doctor.ExitError
	}
	return code
}

// Release builds set version via -ldflags; `go install …@vX` only records it in the build info.
func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "dev"
}

func exitOnInterrupt(u *ui.UI) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signals
		u.Printf("\n")
		u.Doc("Interrupted. Nothing is half-applied: every treatment is a single API call. Goodbye.")
		os.Exit(ui.ExitInterrupted)
	}()
}
