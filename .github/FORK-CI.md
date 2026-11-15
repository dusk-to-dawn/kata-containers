# CI of this fork

This fork runs CI only when someone dispatches it: once per candidate commit,
and twice more for the commit that is released. Nothing runs on push, pull
request, schedule, tag or release. The rules below exist because GitHub
disabled the workflows of another fork of ours, a fork of TopoLVM, after the
workflows inherited from upstream burnt about 1,900 job-minutes in a day.

## Branches and workflows

- **`fixes-4.3.1`** is upstream's tag `4.3.1` plus this fork's fix, then
  fork-only commits that delete every workflow and the Dependabot
  configuration inherited from upstream and add `fork-ci.yaml`. It has to be
  the default branch: GitHub dispatches a workflow only if its file is on the
  default branch.
- There is no other branch. A branch made from upstream carries upstream's
  workflows, scheduled ones included (rules 5 and 6).

| workflow | file | runs on | does |
|---|---|---|---|
| Fork CI | `.github/workflows/fork-ci.yaml` | dispatch only; inputs `ref` (full commit SHA), `extension` (`none`, `build`, `push`) and `control` (boolean) | job `test` always; jobs `preflight`, `extension` and `publish` as asked |

**Jobs.**
- `test` (at most 20 minutes): checks the inputs, and that `ref` is in the
  history of the dispatched branch and based on 4.3.1. It builds the shim at
  `ref` and runs the regression test `TestUpdateCpuSetEnablesCpusetController`
  as root: it must pass, and neither it nor its subtests may be skipped. It
  then reverses the fix outside the tests in the working tree, and the test
  must fail with `cpuset.cpus: permission denied` and
  `cpuset.mems: permission denied`, which shows it detects the bug. It then
  runs the tests of `pkg/resourcecontrol` as root and as the runner user. Last,
  it runs the core scheduling setting's tests (`CORE_SCHEDULING_TESTS`) as
  root: each must pass, and none may be skipped.
- `preflight` (with `push`, beside `test`; at most 5 minutes): checks that
  `EXTENSION_VERSION` does not exist on ghcr.io yet, before anything is built.
  It has `packages: write`, like `publish`, so that the registry answers it
  the same way, and runs nothing but skopeo.
- `extension` (with `build` or `push`, after `test` and `preflight`; at most
  45 minutes): builds the Talos `kata-containers` extension from
  `siderolabs/extensions` at `v1.14.6` (pinned by commit), with their
  `docker-kata-containers` make target, for `linux/amd64`, as
  `ghcr.io/dusk-to-dawn/kata-containers:<EXTENSION_VERSION>`.
  The patch is the diff of `src/` between 4.3.1 and `ref`; the job refuses a
  `ref` that changes anything under `src/` outside `src/runtime`, because the
  recipe builds only the Go shim from source and takes everything else from
  the `kata-go-static` tarball. The patch is added to their recipe the way
  their other extensions add patches, and the recipe's version becomes
  `EXTENSION_VERSION`, which is what `talosctl get extensions` shows. Before
  building, the job checks that the three images Sidero's build pulls by tag
  (the bldr frontend, the tools image that compiles the shim, and the
  extensions validator) still resolve to the digests in `SIDERO_*_PIN`, and
  stops if one has moved (see below). The image is then compared, file by
  file (path, type, mode, sha256, owner and extended attributes), with the
  official image that Talos v1.14.6 lists,
  `ghcr.io/siderolabs/kata-containers:4.3.1@sha256:dfd94e72…`. The job fails
  unless the file lists are equal, only the shim and `manifest.yaml` differ,
  the shim does differ, keeps its mode and owner and holds every string of
  `FIX_MARKER` (one per change) that the official shim lacks, and the
  manifest differs only in its version. The comparison is uploaded as the
  artifact `comparison`. With `control`, the job also builds the recipe
  without the fix and reports whether that shim is byte-identical to the
  official one, also when the comparison above failed; this reports on the
  reproducibility of Sidero's build and never fails the job.
- `publish` (with `push`, after `extension`; at most 10 minutes): pushes the
  exact image the `extension` job compared, and refuses a version that exists
  already. With `preflight`, it is the only job with `packages: write`.

## Cost

Estimates, in job-minutes on GitHub-hosted runners. Nothing has run yet;
replace them with the figures of the first runs.

| entry point | jobs | job-minutes |
|---|---|---|
| `extension: none` | test (~8) | ~8 |
| `extension: build` | test, extension (~17) | ~25 |
| `extension: build`, `control: true` | test, extension with control build (~24) | ~32 |
| `extension: push` | test, extension (~17), publish (~3); preflight (~1) | ~28 + 1 |

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
5. Never use "Sync fork" or the merge-upstream API on any branch, and never
   push a branch named `main` or `stable-*`, or any upstream tag: upstream's
   trees carry upstream's workflows, and the push would run them.
6. Never push a branch whose tree has upstream's workflows, and do not open
   pull requests in this fork.

**Budget: at most about 100 job-minutes per release.** A release as above
(rules 2 and 3) costs about 8 + 32 + 28 = 68 job-minutes, plus about 1 for
the preflight; each earlier candidate adds 8.

## Releasing

1. If the patch changed since the last release, bump `EXTENSION_VERSION` in
   `fork-ci.yaml` (`4.3.1-fix.<n+1>`). A published version is never reused.
   A new change also adds its own string to `FIX_MARKER` (`;`-separated), a
   string that only the new code puts into the shim, and it must stay under
   `src/runtime`, the only part of `src/` the extension builds.
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

## When a Sidero image tag has moved

The `extension` job stops with "… now resolves to …" when one of the tags in
`SIDERO_BLDR_PIN`, `SIDERO_TOOLS_PIN` or `SIDERO_VALIDATOR_PIN` points to
another image than the pinned digest. Nothing is built.

1. Resolve the new digest of each tag, for example
   `skopeo inspect --raw docker://ghcr.io/siderolabs/tools:<tag> | sha256sum`.
2. Look at what changed: the new image's creation time and labels
   (`skopeo inspect`), and Sidero's commits and releases for that tag. Use the
   new digest only if the change is explained.
3. Update the pins in `fork-ci.yaml` in a commit, and treat it as a new
   candidate. The next `build` needs `control: true`, so that the control
   build shows whether the new images still reproduce the official shim.

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
   `OFFICIAL_IMAGE` and `OFFICIAL_AMD64_MANIFEST`, and the three
   `SIDERO_*_PIN` values (the frontend in the new tag's `Pkgfile`, `TOOLS` in
   its `Makefile`, the validator in `internal/base/pkg.yaml`, each with its
   digest). Take the official image's digest from the `image-digests` file in
   `ghcr.io/siderolabs/extensions:<Talos version>`, not from the image's tag:
   Sidero rebuilds images under the same tag, and `kata-containers:4.3.1`
   already points to a newer build than the one Talos v1.14.6 lists.
4. Make the new branch the default branch, and treat it as a new candidate.

## Not yet run

The workflow has not run. What only the first runs can show is listed in the
report that came with this branch; check those points in the first runs'
logs, and replace the estimates under Cost.
