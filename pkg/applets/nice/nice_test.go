package nice_test

import (
	"testing"

	"goblin/pkg/applets/nice"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestNice(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			Name:     "invalid",
			Args:     []string{"-n", "bad", "echo"},
			WantCode: core.ExitUsage,
			WantErr:  "invalid number",
		},
	}
	testutil.RunAppletTests(t, nice.Run, tests)
}
