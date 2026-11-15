# This fork

`dusk-to-dawn/kata-containers` is a fork of
[kata-containers/kata-containers](https://github.com/kata-containers/kata-containers).
It exists to build the Talos `kata-containers` system extension with two
changes that upstream's 4.3.1 lacks, until an upstream release and an official
Talos extension carry them.

## The change against 4.3.1

Branch `fixes-4.3.1` is upstream's tag `4.3.1`
(`03cf2ec06a5d302f23af811744f6bcea49988bfb`) plus two changes to the Go
runtime. The first is the commit "runtime: enable the cpuset controller before
writing a sandbox cpuset":

- **The bug.** Since 4.2.0 the Go runtime uses containerd/cgroups v3, which
  enables the `cpuset` controller for a new cgroup only when the cgroup is
  created with a list of CPUs or memory nodes. The sandbox cgroup
  `kata_<id>` is created from the pause container's spec, which has none. On a
  cgroup v2 host, the first container that carries a CPU set then fails with
  `open …/kata_<id>/cpuset.cpus: permission denied`. With the kubelet's static
  CPU manager, that is every container.
- **The fix.** Before `UpdateCpuSet` writes `cpuset.cpus` or `cpuset.mems`
  of the sandbox cgroup, it checks the sandbox cgroup's `cgroup.controllers`.
  If `cpuset` is missing, it writes `+cpuset` into the
  `cgroup.subtree_control` of each ancestor, from the root cgroup down to the
  parent, which changes nothing where `cpuset` is enabled already. It writes
  nothing when `cpuset` is there; for a cgroup managed by systemd it returns
  a clear error instead of writing. The file is
  `src/runtime/pkg/resourcecontrol/cgroups.go`, with a regression test beside
  it.

The second is the commit "runtime: add a core scheduling setting for Cloud
Hypervisor":

- **Why.** Cloud Hypervisor from v52 gives each VM's vCPU threads one core
  scheduling cookie by default, and Kata 4.3.1 takes that default with no
  setting for it. Measured on our workers, Pods on exclusive CPUs lose storage
  throughput when a VM's vCPUs may not share a core with its own virtiofsd
  threads.
- **The setting.** `core_scheduling` in the `[hypervisor.clh]` section of
  `configuration.toml` takes Cloud Hypervisor's own values, `"Vm"`, `"Vcpu"`
  or `"Off"`, and refuses any other at load with `invalid core_scheduling`.
  Without the key nothing changes: the request is upstream's, with Cloud
  Hypervisor's default (`"Vm"`). The template carries the key commented out.

The commits around them only concern this fork: they delete upstream's workflows
and Dependabot configuration, add a CI that runs only when dispatched, and add
this file and `.github/FORK-CI.md`.

## Telling this build from upstream's

On a Talos node with the extension built here:

- `talosctl get extensions` shows `kata-containers` with version
  `4.3.1-fix.<n>` instead of `4.3.1`;
- `/usr/local/bin/containerd-shim-kata-v2` has a different sha256 from the
  official extension's shim (`4fcf96539b8bb1f8…` in
  `ghcr.io/siderolabs/kata-containers:4.3.1@sha256:dfd94e72…`), and it contains
  the strings `failed to enable the cpuset controller` and
  `invalid core_scheduling`, which the official one does not;
- `containerd-shim-kata-v2 --version` does not tell them apart: both say
  version `4.3.1`. The `VERSION` file is unchanged, and the extension is built
  from upstream's source tarball, so no commit is recorded.

## Upstream

Nothing is opened upstream from this fork: no issue, pull request or comment.
The fix commit is in upstream's format, apart from the trailers that upstream
requires: `Fixes: #<issue>`, `Signed-off-by:`, and `Generated-By:` or
`Assisted-By:` under the OpenInfra AI policy. Its owner adds them before
sending it, should they decide to.

How this fork's CI runs, what it costs and how to release is in
[.github/FORK-CI.md](.github/FORK-CI.md).
