// Copyright (c) 2026 Kata Contributors
//
// SPDX-License-Identifier: Apache-2.0
//

package resourcecontrol

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/containerd/cgroups/v3"
	cgroup2 "github.com/containerd/cgroups/v3/cgroup2"
	ktu "github.com/kata-containers/kata-containers/src/runtime/pkg/katatestutils"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"
)

func TestUpdateCpuSetEnablesCpusetController(t *testing.T) {
	tc := ktu.NewTestConstraint(false)
	if tc.NotValid(ktu.NeedRoot()) {
		t.Skip(ktu.TestDisabledNeedRoot)
	}

	if cgroups.Mode() != cgroups.Unified {
		t.Skip("test requires cgroup v2 (unified mode)")
	}

	testUpdateCpuSetEnablesCpusetController(t, fmt.Sprintf("/kata-tests-cpuset-%d", os.Getpid()))
}

// testUpdateCpuSetEnablesCpusetController creates, below the cgroup v2 path
// base, a pod cgroup whose cgroup.subtree_control lacks cpuset and, in it, a
// sandbox cgroup created the way the sandbox's is: with a CPU section but no
// list of CPUs or memory nodes. UpdateCpuSet must then enable cpuset in the
// pod cgroup and write the sandbox cgroup's cpuset.cpus or cpuset.mems.
func testUpdateCpuSetEnablesCpusetController(t *testing.T, base string) {
	baseDir := filepath.Join(unifiedMountpoint, base)
	if !slices.Contains(readCgroupFields(t, filepath.Dir(baseDir), "cgroup.controllers"), "cpuset") {
		t.Skipf("the cpuset controller is not available in %s", filepath.Dir(baseDir))
	}

	if err := os.Mkdir(baseDir, 0o755); err != nil {
		t.Skipf("cannot create a test cgroup: %v", err)
	}
	t.Cleanup(func() { removeTestCgroup(t, baseDir) })

	for _, tt := range []struct {
		name string
		cpus bool
		mems bool
	}{
		{name: "cpus", cpus: true},
		{name: "mems", mems: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			podPath := filepath.Join(base, "pod-"+tt.name)
			podDir := filepath.Join(unifiedMountpoint, podPath)
			sandboxPath := filepath.Join(podPath, "kata_sandbox")
			sandboxDir := filepath.Join(unifiedMountpoint, sandboxPath)

			if err := os.Mkdir(podDir, 0o755); err != nil {
				t.Fatalf("cannot create the pod cgroup: %v", err)
			}
			t.Cleanup(func() { removeTestCgroup(t, podDir) })

			// Make cpuset available to the pod cgroup, but leave it out of the
			// pod cgroup's own cgroup.subtree_control.
			pod, err := cgroup2.Load(podPath, cgroup2.WithMountpoint(unifiedMountpoint))
			assert.NoError(err)
			if err := pod.ToggleControllers([]string{"cpu", "cpuset"}, cgroup2.Enable); err != nil {
				t.Skipf("cannot make cpuset available to %s: %v", podDir, err)
			}

			var cpus, mems string
			if tt.cpus {
				cpus = firstListEntry(t, podDir, "cpuset.cpus.effective")
			}
			if tt.mems {
				mems = firstListEntry(t, podDir, "cpuset.mems.effective")
			}

			// The sandbox cgroup gets the CPU section of the pause container's
			// spec, which has no list of CPUs or memory nodes.
			sandbox, err := NewResourceController(sandboxPath, &specs.LinuxResources{CPU: &specs.LinuxCPU{}})
			if !assert.NoError(err) {
				return
			}
			t.Cleanup(func() { removeTestCgroup(t, sandboxDir) })

			assert.NotContains(readCgroupFields(t, podDir, "cgroup.subtree_control"), "cpuset")
			assert.NoFileExists(filepath.Join(sandboxDir, "cpuset.cpus"))

			assert.NoError(sandbox.UpdateCpuSet(cpus, mems))

			assert.Contains(readCgroupFields(t, podDir, "cgroup.subtree_control"), "cpuset")
			if tt.cpus {
				assert.Equal([]string{cpus}, readCgroupFields(t, sandboxDir, "cpuset.cpus"))
			}
			if tt.mems {
				assert.Equal([]string{mems}, readCgroupFields(t, sandboxDir, "cpuset.mems"))
			}
		})
	}
}

// readCgroupFields returns the whitespace-separated fields of a cgroup file.
func readCgroupFields(t *testing.T, dir, file string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatalf("cannot read %s: %v", filepath.Join(dir, file), err)
	}
	return strings.Fields(string(b))
}

// firstListEntry returns the first CPU or memory node of a cpuset list file,
// such as "0" for "0-3,8".
func firstListEntry(t *testing.T, dir, file string) string {
	t.Helper()
	fields := readCgroupFields(t, dir, file)
	if len(fields) == 0 {
		t.Skipf("%s is empty in %s", file, dir)
	}
	return strings.FieldsFunc(fields[0], func(r rune) bool { return r == ',' || r == '-' })[0]
}

func removeTestCgroup(t *testing.T, dir string) {
	t.Helper()
	if err := os.Remove(dir); err != nil && !os.IsNotExist(err) {
		t.Errorf("cannot remove the test cgroup %s: %v", dir, err)
	}
}
