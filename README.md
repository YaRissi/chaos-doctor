# The Chaos Doctor

An interactive CLI that examines a Kubernetes app step by step, explains in one sentence what is wrong, shows the evidence, and offers to heal it. If it cannot name the fault, it says so and tells you where to look next.

## Setup

Prerequisites: docker, Go 1.26+, [kind](https://kind.sigs.k8s.io/), kubectl, helm. Optional: [just](https://github.com/casey/just); with Nix, `nix develop` provides everything except docker.

```bash
kind create cluster --config kind.yaml --wait 120s
helm install web chart -n clinic --create-namespace --wait
```

This deploys namespace `clinic` with deployment and service `web` (nginx, 2 replicas) mounting configmap `web-config`. `just up`, `just reset` and `just down` do the same with less typing.

## Run

```bash
go install github.com/YaRissi/chaos-doctor/cmd/doctor@latest   # or use ./doctor.sh from a checkout
./doctor.sh                    # asks which namespace and app to examine
./doctor.sh -n clinic -a web   # or pass them directly
./doctor.sh -n clinic -a web --auto
```

`--auto` heals without asking and refuses whenever it does not know the correct value. Without a terminal and without `--auto` it only diagnoses. Exit codes: `0` healthy, `3` healed, `2` not healed or unknown, `4` could not examine, `1` usage error, `130` interrupted. Prebuilt binaries are attached to each GitHub release.

## How it works

The checks run in dependency order (`internal/diseases/checks.go`): ConfigMaps exist → replicas > 0 → newest pods Ready (and if not, why) → service selector → targetPort → ready endpoints → a real request through the service. The first problem is the root cause; everything after it would only be a symptom. After a treatment the doctor examines the patient again from the top, which is how several faults at once get healed.

Each disease lives in one file in `internal/diseases/`, named like its diagnosis (`Diagnosis [bad-image]` → `bad_image.go`), with its detection and its treatment. `internal/cluster` reads the cluster, `internal/doctor` runs the loop and applies treatments, `cmd/doctor` is the CLI. Correct values come from the Helm release that deployed the app, otherwise from the last healthy ReplicaSet, otherwise the doctor asks; it never invents one.

## Faults tested

Each fault was injected by hand on a fresh install; the doctor ran with `--auto`, and healed faults were re-checked as healthy afterwards.

| Fault | How I broke it | Result |
| --- | --- | --- |
| Image tag does not exist | `kubectl -n clinic set image deployment/web web=nginx:no-such-tag` | healed: image from the Helm release |
| Scaled to 0 | `kubectl -n clinic scale deployment/web --replicas=0` | healed: scaled to the Helm replica count |
| Service selector broken | `kubectl -n clinic patch service web -p '{"spec":{"selector":{"app":"wrong"}}}'` | healed: selector set to the deployment's own |
| ConfigMap deleted, pods still running | `kubectl -n clinic delete configmap web-config` | healed: re-created from the Helm release |
| ConfigMap deleted, pods restarted | the above plus `kubectl -n clinic rollout restart deployment/web` | healed: re-created from the Helm release |
| Two faults at once | scaled to 0 and ConfigMap deleted | healed in two rounds |
| Readiness probe failing | readiness probe path patched to `/nope` | diagnosed; suggests `kubectl rollout undo` |
| OOMKilled | `kubectl -n clinic set resources deployment/web -c web --limits=memory=6Mi --requests=memory=6Mi` | diagnosed; suggests `kubectl rollout undo` |
| Unschedulable | `nodeSelector: {disk: ssd}` patched into the pod template | diagnosed; suggests `kubectl rollout undo` |
| Wrong targetPort | service `targetPort` patched to `8080` | diagnosed; suggests `kubectl edit service` |
| Pods forbidden by a quota | `kubectl -n clinic create quota no-pods --hard=pods=0` plus a restart | `I don't know`, with what it saw and where to look |

Also tested: cluster not running, unknown namespace or app, garbage at every prompt, Ctrl-C, no terminal.

Tested with: Go 1.26.7, client-go 0.37.1, kubectl 1.37.1, Kubernetes 1.36.1 (kind 0.32.0), helm 4.3.0, bash 5.3 (only for the `doctor.sh` wrapper).

## Why Go instead of bash

The challenge allows another language. Go is on the team's tech radar, client-go is the same client kubectl is built on, and typed Kubernetes objects are easier to read and defend line by line than kubectl output parsed with jq. `doctor.sh` keeps the expected entry point.
