package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

const (
	maxTries     = 3
	evidenceSize = 200
)

type ui struct {
	in                                         *bufio.Reader
	out                                        io.Writer
	auto, interactive                          bool
	red, green, yellow, blue, dim, bold, reset string
}

func newUI(auto bool) *ui {
	u := &ui{in: bufio.NewReader(os.Stdin), out: os.Stdout, auto: auto, interactive: isTerminal(os.Stdin)}
	if isTerminal(os.Stdout) && os.Getenv("NO_COLOR") == "" {
		u.red, u.green, u.yellow, u.blue = "\033[31m", "\033[32m", "\033[33m", "\033[34m"
		u.dim, u.bold, u.reset = "\033[2m", "\033[1m", "\033[0m"
	}
	return u
}

func isTerminal(f *os.File) bool { return term.IsTerminal(int(f.Fd())) }

func (u *ui) readOnly() bool { return !u.auto && !u.interactive }

func (u *ui) em(s string) string { return u.bold + s + u.reset }

func (u *ui) doc(format string, a ...any) {
	u.printf("%s%sDr. Kube:%s %s\n", u.blue, u.bold, u.reset, fmt.Sprintf(format, a...))
}

func (u *ui) printf(format string, a ...any) { _, _ = fmt.Fprintf(u.out, format, a...) }

func (u *ui) step(s string) { u.printf("\n%s» %s%s\n", u.bold, s, u.reset) }

func (u *ui) ok(format string, a ...any) { u.mark(u.green+"✓", format, a...) }

func (u *ui) bad(format string, a ...any) { u.mark(u.red+"✗", format, a...) }

func (u *ui) warn(format string, a ...any) { u.mark(u.yellow+"!", format, a...) }

func (u *ui) mark(symbol, format string, a ...any) {
	u.printf("  %s%s %s\n", symbol, u.reset, fmt.Sprintf(format, a...))
}

func (u *ui) evidence(s string) { u.printf("    %s│ %s%s\n", u.dim, truncate(s), u.reset) }

func truncate(s string) string {
	if len(s) > evidenceSize {
		return s[:evidenceSize] + "…"
	}
	return s
}

func (u *ui) readLine(prompt string) string {
	u.printf("%s?%s %s", u.bold, u.reset, prompt)
	line, err := u.in.ReadString('\n')
	if err != nil && line == "" {
		u.printf("\n")
		u.doc("Input closed, so I'll stop here.")
		os.Exit(exitInterrupted)
	}
	return strings.TrimSpace(line)
}

func (u *ui) confirm(question string) bool {
	switch {
	case u.auto:
		u.printf("%s?%s %s yes (--auto)\n", u.bold, u.reset, question)
		return true
	case u.readOnly():
		u.printf("%s?%s %s no (no terminal: diagnose-only, use --auto to heal)\n", u.bold, u.reset, question)
		return false
	}
	for range maxTries {
		switch strings.ToLower(u.readLine(question + " ")) {
		case "y", "yes":
			return true
		case "n", "no":
			return false
		}
		u.warn("Please answer y or n.")
	}
	u.warn("I'll take that as a no.")
	return false
}

func (u *ui) ask(question, fallback string, valid func(string) bool) (string, bool) {
	if u.auto || u.readOnly() {
		return fallback, fallback != ""
	}
	hint := ""
	if fallback != "" {
		hint = "[" + fallback + "] "
	}
	for range maxTries {
		v := u.readLine(question + " " + hint)
		if v == "" {
			v = fallback
		}
		if valid(v) {
			return v, true
		}
		u.warn("That doesn't look right.")
	}
	return "", false
}
