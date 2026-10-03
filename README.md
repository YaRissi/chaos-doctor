# The Chaos Doctor

`doctor` is an interactive command-line tool, written in Go on top of client-go, that examines a Kubernetes app, explains in plain English what is wrong, quotes the evidence it based that on, and offers to heal it. When it cannot identify the fault it says so, shows what it saw and tells you where a human should look next.

```
» How are the pods of the newest rollout doing?
  ✗ 0 of 1 pods of 'web-6688757845' are Ready.

Diagnosis [bad-image]: The rollout uses image 'nginx:no-such-tag', which does not exist in the registry, so new pods can't start.
    │ ImagePullBackOff: Back-off pulling image "nginx:no-such-tag": ErrImagePull: rpc error: code = NotFound ...
Dr. Kube: Treatment based on: Helm release 'web' (revision 1) specifies nginx:1.27-alpine.
  $ kubectl -n clinic set image deployment/web web=nginx:1.27-alpine
? Shall I heal it? (y/n)
```

## Quick start

Prerequisites: docker, Go 1.26+, [kind](https://kind.sigs.k8s.io/), kubectl and helm. Optional: [just](https://github.com/casey/just). With Nix, `nix develop` provides everything except docker.

```bash
just up                       # kind cluster "clinic" + demo app (namespace clinic, deployment/service web, configmap web-config)
./doctor.sh                       # asks which namespace and app to examine (wraps go run ./doctor)
./doctor.sh -n clinic -a web      # or pass them directly
just reset                    # reinstall the demo app
just down                     # delete the cluster
```

Without `just`:

```bash
kind create cluster --config kind.yaml --wait 120s
helm install web chart -n clinic --create-namespace --wait
kubectl -n clinic set image deployment/web web=nginx:no-such-tag   # break something
./doctor.sh -n clinic -a web
```

### Options

| Option | Meaning |
| --- | --- |
| `-n`, `--namespace` | namespace to examine; prompted for if omitted |
| `-a`, `--app` | deployment to examine; discovered and offered as a menu if omitted |
| `--auto` | heal without asking, but refuse whenever the correct value is unknown |

The service is expected to have the same name as the deployment. Without a terminal and without `--auto` the doctor only diagnoses and never changes anything.

Exit codes: `0` healthy, `3` healed, `2` not healed or unknown fault, `4` could not examine (cluster down, namespace or app missing), `1` usage error, `130` interrupted. `go run` reports every failure as exit code 1, so scripts that need these codes should run a built binary (`go build -o bin/doctor ./doctor && bin/doctor --auto ...`).

## How the doctor thinks

Every check prints what it is looking at and what it found. The checks run in dependency order, so the first finding is the one to treat first:

1. **Cluster** – is the API server ready, and which server is this?
2. **ConfigMaps** – does every ConfigMap the pod template mounts or reads still exist?
3. **Replicas** – does the deployment ask for any pods at all?
4. **Newest rollout** – are the pods of the current ReplicaSet Ready? If not, why: image pull, OOMKilled, unschedulable, failing readiness probe.
5. **Service** – does its selector match the pod labels, and does its `targetPort` exist on the container?
6. **Endpoints** – does the service have ready endpoints (EndpointSlices)?
7. **Traffic** – does a real request through the service (via the API server's service proxy) get an answer?

It stops at the first problem, because everything after it in the chain would only be a downstream symptom, and treats one disease at a time: one-sentence diagnosis, the evidence, the treatment as its `kubectl` equivalent, `Shall I heal it? (y/n)`, then it examines the patient again from the top. That loop is what handles several faults at once; it gives up after five rounds.

**What it heals.** The four faults from the challenge: it scales the deployment up, sets the image back, puts the service selector back to the deployment's own selector, or re-creates the deleted ConfigMap. Every treatment is a single API call, shown as the equivalent `kubectl` command before it runs. For the other faults it detects (OOMKilled, unschedulable, failing readiness probe, wrong targetPort) it explains the cause and suggests the command, but does not change anything, because the right memory limit, probe or port is a decision for a human.

**Where the correct values come from.** The doctor never invents a value. Replica count, image and ConfigMap content come from the Helm release that deployed the app (read from the release Secret, the same data `helm get manifest` shows); the image can also come from the last ReplicaSet whose pods were healthy; otherwise it asks you. In `--auto` mode it refuses when it has no source. It always prints which source it used.

**What "unknown" looks like.** If something is off but matches none of the faults below, the doctor prints `I don't know what is wrong`, lists everything it observed, and the commands a human should run next (logs, events, quotas, network policies, node health).

### Code layout

Everything is one Go `package main` in `doctor/`; every file has one job and none is longer than about 120 lines.

The examination:

| File | Purpose |
| --- | --- |
| `doctor/exam.go` | the ordered list of checks and of pod diseases, plus the checks that cannot name a disease (rollout, endpoints, request) |
| `doctor/doctor.go` | the loop: snapshot → checks → diagnosis → treatment → examine again; the "I don't know" report |
| `doctor/snapshot.go` | reads everything the checks need, once per round |
| `doctor/treatment.go` | how a treatment is confirmed, applied and waited for |

One file per disease, named like the diagnosis on screen (`Diagnosis [bad-image]` → `bad_image.go`), each with its detection and, where it has one, its treatment:

| File | Detects | Treatment |
| --- | --- | --- |
| `doctor/missing_config.go` | a referenced ConfigMap is gone | re-create it from the Helm release |
| `doctor/scaled_to_zero.go` | `.spec.replicas == 0` | scale to the Helm replica count |
| `doctor/bad_image.go` | image tag not found in the registry | set the image from Helm or the last healthy ReplicaSet |
| `doctor/oom_killed.go` | container OOMKilled and crash-looping | suggestion only |
| `doctor/unschedulable.go` | pod Pending, no node fits | suggestion only |
| `doctor/readiness_failing.go` | readiness probe fails | suggestion only |
| `doctor/bad_selector.go` | service selector does not match the pods | set it to the deployment's own selector |
| `doctor/bad_target_port.go` | service targets a port the container does not declare | suggestion only |

Plumbing: `main.go` (CLI flags, exit codes, Ctrl-C), `setup.go` (connect, pick namespace and app), `helm.go` (desired state from the Helm release Secret), `k8s.go` (small helpers shared by several files), `ui.go` (output and prompts).

Checks never talk to the cluster: `snapshot.go` reads, `treatment.go` writes, everything in between is plain functions over data.

### Adding a fault

1. Create one file named after the disease, e.g. `doctor/crash_loop.go`, with a function that returns a `finding` (symptom, disease, one-sentence cause, evidence).
2. Register it with one line in `doctor/exam.go`: in `podDiseases` if it explains why a pod is not Ready, otherwise in `checks` at the position where it belongs in the dependency chain.
3. If it needs data the snapshot does not have yet, add a field to `snapshot` and fill it in `takeSnapshot`.
4. To make it healable, set `heal` to a function in the same file that returns a `treatment`; otherwise set `suggest` to the command a human should run.

## Faults tested

I broke the app by hand with each command below on a fresh install, ran the built doctor with `--auto`, and checked the diagnosis and exit code. For the healed faults I ran the doctor again to confirm the app was healthy afterwards.

| Fault | How I broke it | What the doctor looks at | Treatment |
| --- | --- | --- | --- |
| Image tag does not exist | `kubectl -n clinic set image deployment/web web=nginx:no-such-tag` | newest pods waiting in `ErrImagePull`/`ImagePullBackOff` with a registry `not found` | `kubectl set image` with the image from the Helm release or the last healthy ReplicaSet |
| Scaled to 0 | `kubectl -n clinic scale deployment/web --replicas=0` | `.spec.replicas == 0` (Kubernetes still reports `Available=True`) | `kubectl scale` to the Helm replica count |
| Service selector broken | `kubectl -n clinic patch service web -p '{"spec":{"selector":{"app":"wrong"}}}'` | selector vs pod template labels | `kubectl patch` the selector back to the deployment's own |
| ConfigMap deleted, pods running | `kubectl -n clinic delete configmap web-config` | referenced ConfigMap missing, `FailedMount` events; running pods keep serving their old copy | `kubectl create` it from the Helm release |
| ConfigMap deleted, pods restarted | the above plus `kubectl -n clinic rollout restart deployment/web` | new pods stuck in `ContainerCreating` with `FailedMount` | `kubectl create` it from the Helm release |
| Readiness probe failing | readiness probe path patched to `/nope` | newest pods Running but not Ready, `Unhealthy: Readiness probe failed` | diagnosis only; suggests `kubectl rollout undo` |
| OOMKilled | `kubectl -n clinic set resources deployment/web -c web --limits=memory=6Mi --requests=memory=6Mi` | `lastState.terminated.reason == OOMKilled`, `CrashLoopBackOff` | diagnosis only; suggests `kubectl rollout undo` |
| Unschedulable | `nodeSelector: {disk: ssd}` patched into the pod template | `PodScheduled=False`, reason `Unschedulable`, scheduler message | diagnosis only; suggests `kubectl rollout undo` |
| Wrong targetPort | service `targetPort` patched to `8080` | targetPort vs the container's declared ports | diagnosis only; suggests `kubectl edit service` |
| Two faults at once | scaled to 0 and ConfigMap deleted | both found; ConfigMap restored first, then scaled up | two rounds |
| Pods forbidden by a quota | `kubectl -n clinic create quota no-pods --hard=pods=0` plus a restart | not a known fault: must answer `I don't know` | none, by design |

Tested with: Go 1.26.7, client-go 0.37.1, kubectl 1.37.1 against Kubernetes 1.36.1 (kind 0.32.0, single node), helm 4.3.0. Bash is only used for the three-line `doctor.sh` wrapper (bash 5.3).

## Decisions

| Question | Choice | Why |
| --- | --- | --- |
| Language | Go with client-go instead of the suggested bash + kubectl | The challenge allows another language; Go is on the team's Adopt list, client-go is the same client kubectl is built on, and typed objects are easier to read and defend line by line than kubectl output parsed with jq. `doctor.sh` keeps the expected entry point. |
| CLI | cobra | POSIX short and long flags and generated `--help`, the convention for kubectl-style Go tools. |
| Demo app packaging | a minimal Helm chart | The release stored in the cluster gives the doctor a trustworthy "what should this be" for healing. |
| Endpoints | EndpointSlices | The Endpoints API is deprecated since Kubernetes 1.33. |
| Traffic check | API server service proxy | One synchronous call, no helper pod or port-forward; it does not test DNS, kube-proxy or NetworkPolicies. |
| Healing | one client-go API call per treatment, shown as its `kubectl` equivalent | One consistent client for reading and writing; the printed command lets a human repeat or review the fix. |
| Scope of healing | only the four required faults | For memory limits, probes, scheduling and ports the right value is a human decision; the doctor explains and suggests instead of guessing. |

## Limitations

- One container, one service port: the doctor looks at the first of each.
- Reading the Helm release needs permission to list Secrets in the namespace; without it the doctor falls back to the previous ReplicaSet or asks.
- Healing through the API drifts from git/Helm; the durable fix belongs in the chart. Under a GitOps controller (Flux, Argo CD) the controller may undo the fix.
- Events expire after an hour, so the doctor bases diagnoses on object state and uses events as supporting evidence. The exception is a failing readiness probe, which Kubernetes only reports as an `Unhealthy` event; after an hour without a new probe failure that case falls back to "I don't know".
- A missing image is recognised by the registry saying "not found"; other registries may word it differently, and then the doctor says "I don't know" instead of guessing.
- Not checked: application logs, node health, quotas and limit ranges, NetworkPolicies, ingress and DNS outside the cluster. The unknown-fault report points at each of these.

## How AI was used

I used Claude as a pair: to research how Kubernetes reports each failure, to draft code, and to challenge the design. Every detection rule is based on what I measured on a live kind cluster first, not on documentation alone, and every fault in the table above was run against the finished doctor.
