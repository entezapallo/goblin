package taskset_test

import (
	"testing"

	"goblin/pkg/applets/taskset"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestTaskset(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid_mask",
			Args:     []string{"bad", "echo"},
			WantCode: core.ExitSuccess,
		},
	}
	testutil.RunAppletTests(t, taskset.Run, tests)
}
