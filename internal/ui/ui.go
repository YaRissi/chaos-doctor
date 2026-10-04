// Package ui prints the doctor's narration and reads validated answers from the terminal.
package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	MaxTries        = 3
	ExitInterrupted = 130
	evidenceSize    = 200
)

type UI struct {
	in                                         *bufio.Reader
	Out                                        io.Writer
	Auto, Interactive                          bool
	Red, Green, Yellow, Blue, Dim, Bold, Reset string
}

func New(auto bool) *UI {
	u := &UI{in: bufio.NewReader(os.Stdin), Out: os.Stdout, Auto: auto, Interactive: isTerminal(os.Stdin)}
	if isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == "" {
		u.Red, u.Green, u.Yellow, u.Blue = "\033[31m", "\033[32m", "\033[33m", "\033[34m"
		u.Dim, u.Bold, u.Reset = "\033[2m", "\033[1m", "\033[0m"
	}
	return u
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func (u *UI) ReadOnly() bool { return !u.Auto && !u.Interactive }

func (u *UI) Em(s string) string { return u.Bold + s + u.Reset }

func (u *UI) Printf(format string, a ...any) { _, _ = fmt.Fprintf(u.Out, format, a...) }

func (u *UI) Doc(format string, a ...any) {
	u.Printf("%s%sDr. Kube:%s %s\n", u.Blue, u.Bold, u.Reset, fmt.Sprintf(format, a...))
}

func (u *UI) Step(s string) { u.Printf("\n%s» %s%s\n", u.Bold, s, u.Reset) }

func (u *UI) OK(format string, a ...any) { u.mark(u.Green+"✓", format, a...) }

func (u *UI) Bad(format string, a ...any) { u.mark(u.Red+"✗", format, a...) }

func (u *UI) Warn(format string, a ...any) { u.mark(u.Yellow+"!", format, a...) }

func (u *UI) mark(symbol, format string, a ...any) {
	u.Printf("  %s%s %s\n", symbol, u.Reset, fmt.Sprintf(format, a...))
}

func (u *UI) Evidence(s string) {
	if len(s) > evidenceSize {
		s = s[:evidenceSize] + "…"
	}
	u.Printf("    %s│ %s%s\n", u.Dim, s, u.Reset)
}

func (u *UI) readLine(prompt string) string {
	u.Printf("%s?%s %s", u.Bold, u.Reset, prompt)
	line, err := u.in.ReadString('\n')
	if err != nil && line == "" {
		u.Printf("\n")
		u.Doc("Input closed, so I'll stop here.")
		os.Exit(ExitInterrupted)
	}
	return strings.TrimSpace(line)
}

func (u *UI) Confirm(question string) bool {
	switch {
	case u.Auto:
		u.Printf("%s?%s %s yes (--auto)\n", u.Bold, u.Reset, question)
		return true
	case u.ReadOnly():
		u.Printf("%s?%s %s no (no terminal: diagnose-only, use --auto to heal)\n", u.Bold, u.Reset, question)
		return false
	}
	for range MaxTries {
		switch strings.ToLower(u.readLine(question + " ")) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		u.Warn("Please answer y or n.")
	}
	u.Warn("I'll take that as a no.")
	return false
}

func (u *UI) Ask(question, fallback string, valid func(string) bool) (string, bool) {
	if u.Auto || u.ReadOnly() {
		return fallback, fallback != ""
	}
	hint := ""
	if fallback != "" {
		hint = "[" + fallback + "] "
	}
	for range MaxTries {
		v := u.readLine(question + " " + hint)
		if v == "" {
			v = fallback
		}
		if valid(v) {
			return v, true
		}
		u.Warn("That doesn't look right.")
	}
	return "", false
}
