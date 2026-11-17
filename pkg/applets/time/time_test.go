package time_test

import (
	"testing"

	"goblin/pkg/applets/time"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestTime(t *testing.T) {
	tests := []testutil.AppletTestCase{
		{
			Name:     "missing",
			Args:     []string{},
			WantCode: core.ExitUsage,
		},
		{
			// time escribe el resumen ("real", "user", "sys") a stderr.
			Name:     "basic",
			Args:     []string{"echo", "hello"},
			WantCode: core.ExitSuccess,
			WantErr:  "real",
		},
	}
	testutil.RunAppletTests(t, time.Run, tests)
}
