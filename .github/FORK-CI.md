# CI of this fork

This fork runs CI only when someone dispatches it, once per candidate commit.
Nothing runs on push, pull request, schedule, tag or release. The rules below
exist because GitHub disabled the workflows of another fork of ours, a fork of
TopoLVM, after the workflows inherited from upstream burnt about 1,900
job-minutes in a day.

## Branches and workflows

- **`fixes-4.3.1`** is upstream's tag `4.3.1` plus this fork's fix, then
  fork-only commits that delete every workflow and the Dependabot
  configuration inherited from upstream and add `fork-ci.yaml`. It has to be
  the default branch: GitHub dispatches a workflow only if its file is on the
  default branch.
- **`main`** is a copy of upstream's default branch from when the fork was
  made. It carries upstream's 65 workflows, scheduled ones included. Nothing
  runs there as long as `main` is not the default branch, no pull request
  targets it and nobody pushes to it.

| workflow | file | runs on | does |
|---|---|---|---|
| Fork CI | `.github/workflows/fork-ci.yaml` | dispatch only; inputs `ref` (full commit SHA), `extension` (`none`, `build`, `push`) and `control` (boolean) | job `test` always; jobs `extension` and `publish` as asked |

**Jobs.**
- `test` (at most 20 minutes): checks the inputs, and that `ref` is in the
  history of the dispatched branch and based on 4.3.1. It builds the shim at
  `ref` and runs the regression test `TestUpdateCpuSetEnablesCpusetController`
  as root: it must pass, and neither it nor its subtests may be skipped. It
  then reverses the fix outside the tests in the working tree, and the test
  must fail with `cpuset.cpus: permission denied` and
  `cpuset.mems: permission denied`, which shows it detects the bug. Last, it
  runs the tests of `pkg/resourcecontrol` as root and as the runner user.
- `extension` (with `build` or `push`, after `test`; at most 45 minutes):
  builds the Talos `kata-containers` extension from `siderolabs/extensions`
  at `v1.14.6` (pinned by commit), with their `docker-kata-containers` make
  target, for `linux/amd64`, as `ghcr.io/dusk-to-dawn/kata-containers:<EXTENSION_VERSION>`.
  The patch is the diff of `src/` between 4.3.1 and `ref`. It is added to
  their recipe the way their other extensions add patches, and the recipe's
  version becomes `EXTENSION_VERSION`, which is what `talosctl get extensions`
  shows. The image is then compared, file by file (path, type, mode, sha256),
  with the official image that Talos v1.14.6 lists,
  `ghcr.io/siderolabs/kata-containers:4.3.1@sha256:dfd94e72…`. The job fails
  unless the file lists are equal, only the shim and `manifest.yaml` differ,
  the shim does differ and holds the fix, and the manifest differs only in its
  version. The comparison is uploaded as the artifact `comparison`. With
  `control`, the job also builds the recipe without the fix and reports
  whether that shim is byte-identical to the official one; this reports on the
  reproducibility of Sidero's build and never fails the job.
- `publish` (with `push`, after `extension`; at most 10 minutes): pushes the
  exact image the `extension` job compared, and refuses a version that exists
  already. It is the only job with `packages: write`.

## Cost

Estimates, in job-minutes on GitHub-hosted runners. Nothing has run yet;
replace them with the figures of the first runs.

| entry point | jobs | job-minutes |
|---|---|---|
| `extension: none` | test (~8) | ~8 |
| `extension: build` | test, extension (~17) | ~25 |
| `extension: build`, `control: true` | test, extension with control build (~24) | ~32 |
| `extension: push` | test, extension (~17), publish (~3) | ~28 |

## Rules

1. Before any dispatch, the local checks pass on the candidate commit:
   `gofmt`, `go vet` and the unit tests of the packages it touches, the shim
   build (`make -C src/runtime SKIP_GO_VERSION_CHECK=y containerd-shim-v2`),
   and `actionlint` on `fork-ci.yaml` if the workflow changed.
2. One dispatch with `extension: none` per candidate commit.
3. For a release, one dispatch with `extension: build` and `control: true`,
   then one with `extension: push`, for the same commit. After the first
   release, the control build is needed only when the Sidero base changes.
4. Nothing else. A failed run is fixed locally, and the fix is a new
   candidate. Re-run a failed job only for an infrastructure failure, once.
5. Never use "Sync fork" on `main`, and never push a branch named `main` or
   `stable-*`, or any upstream tag: their trees carry upstream's workflows,
   and a push would run them.
6. Never push a branch whose tree has upstream's workflows, and do not open
   pull requests in this fork.

**Budget: at most about 100 job-minutes per release.** A release as above
costs about 32 + 28 = 60 job-minutes; two candidates before it add 8 each.

## Releasing

1. If the patch changed since the last release, bump `EXTENSION_VERSION` in
   `fork-ci.yaml` (`4.3.1-fix.<n+1>`). A published version is never reused.
2. Run the local checks, then push `fixes-4.3.1`. Nothing runs.
3. Dispatch Fork CI with `ref` = the full SHA of the branch's head and
   `extension: none`.
4. If it passes, dispatch with the same `ref`, `extension: build` and
   `control: true`. Read the comparison in the run's summary.
5. If it passes, dispatch with the same `ref` and `extension: push`. The run's
   summary prints `ghcr.io/dusk-to-dawn/kata-containers:<version>@sha256:…`.
6. Tag that commit `<version>` (for example `4.3.1-fix.1`) and make a release
   from the tag, with the patch, the `comparison` artifact and the image's
   digest. A tag on this branch runs nothing.

## Rebasing onto a newer upstream

1. Check upstream first. If a release fixes the bug and Sidero ships it in an
   extension for the Talos version in use, stop using this fork.
2. Otherwise, locally: fetch the new upstream tag, create
   `fixes-<version>` from it, and cherry-pick the fix and then the fork-only
   commits. When cherry-picking the commit that deletes the workflows, delete
   `.github/workflows` and `.github/dependabot.yml` of the new tree again:
   upstream may have added files.
3. In `fork-ci.yaml`, update `BASE_COMMIT`, `EXTENSION_VERSION` and its
   pattern, `SIDERO_EXTENSIONS_COMMIT` and `SIDERO_EXTENSIONS_TAG` (the
   `siderolabs/extensions` tag of the Talos release in use), and
   `OFFICIAL_IMAGE` and `OFFICIAL_AMD64_MANIFEST`. Take the official image's
   digest from the `image-digests` file in
   `ghcr.io/siderolabs/extensions:<Talos version>`, not from the image's tag:
   Sidero rebuilds images under the same tag, and `kata-containers:4.3.1`
   already points to a newer build than the one Talos v1.14.6 lists.
4. Make the new branch the default branch, and treat it as a new candidate.

## Not yet run

The workflow has not run. What only the first runs can show is listed in the
report that came with this branch; check those points in the first runs'
logs, and replace the estimates under Cost.
