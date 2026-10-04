# The Chaos Doctor

[![Test](https://github.com/YaRissi/chaos-doctor/actions/workflows/test.yml/badge.svg)](https://github.com/YaRissi/chaos-doctor/actions/workflows/test.yml) [![Release](https://img.shields.io/github/v/release/YaRissi/chaos-doctor)](https://github.com/YaRissi/chaos-doctor/releases)

Examines a Kubernetes app step by step, names the root cause in one sentence and offers to heal it. If it can't name the fault, it says so.

```text
» Does every ConfigMap the pods need still exist?
  ✗ ConfigMap 'web-config' is referenced by the pod template but does not exist.

Diagnosis [missing-config]: ConfigMap 'web-config' was deleted; running pods still serve their old copy, but every new or restarted pod will hang in ContainerCreating.
Dr. Kube: Treatment based on: Helm release 'web' (revision 1), which still contains ConfigMap 'web-config'.
? Shall I heal it? (y/n) y
  ✓ Treatment applied.
…
Dr. Kube: The patient is healthy again.
```

## Setup

Needs docker, Go 1.26+, [kind](https://kind.sigs.k8s.io/), kubectl and helm (`nix develop` provides all but docker).

```bash
kind create cluster --config kind.yaml --wait 120s
helm install web chart -n clinic --create-namespace --wait
```

Deploys deployment and service `web` (nginx, 2 replicas, mounts configmap `web-config`) in namespace `clinic`. With [just](https://github.com/casey/just): `just up`, `just reset`, `just down`.

## Run

```bash
./doctor.sh                           # asks for namespace and app
./doctor.sh -n clinic -a web
./doctor.sh -n clinic -a web --auto   # heal without asking, refuse when unsure
```

Or install it: `go install github.com/YaRissi/chaos-doctor/cmd/doctor@latest`, or grab a binary from [Releases](https://github.com/YaRissi/chaos-doctor/releases).

Without a terminal it only diagnoses. Exit codes: `0` healthy · `3` healed · `2` not healed · `4` can't examine · `1` usage · `130` interrupted.

## Faults tested

Injected by hand on a fresh install, doctor run with `--auto`.

| Fault | Injected with | Result |
| --- | --- | --- |
| Image tag does not exist | `kubectl -n clinic set image deployment/web web=nginx:no-such-tag` | healed |
| Scaled to 0 | `kubectl -n clinic scale deployment/web --replicas=0` | healed |
| Service selector broken | `kubectl -n clinic patch service web -p '{"spec":{"selector":{"app":"wrong"}}}'` | healed |
| ConfigMap deleted, pods running | `kubectl -n clinic delete configmap web-config` | healed |
| ConfigMap deleted, pods restarted | the above + `kubectl -n clinic rollout restart deployment/web` | healed |
| Two faults at once | scaled to 0 + ConfigMap deleted | healed in 2 rounds |
| Readiness probe failing | probe path patched to `/nope` | diagnosed |
| OOMKilled | `kubectl -n clinic set resources deployment/web -c web --limits=memory=6Mi --requests=memory=6Mi` | diagnosed |
| Unschedulable | `nodeSelector: {disk: ssd}` patched into the pod template | diagnosed |
| Wrong targetPort | service `targetPort` patched to `8080` | diagnosed |
| Pods forbidden by a quota | `kubectl -n clinic create quota no-pods --hard=pods=0` + restart | "I don't know" + where to look |

Also tested: cluster down, unknown namespace or app, garbage at prompts, Ctrl-C, no terminal.

**Tested with** Go 1.26.7 · client-go 0.37.1 · kubectl 1.37.1 · Kubernetes 1.36.1 (kind 0.32.0) · helm 4.3.0 · bash 5.3
