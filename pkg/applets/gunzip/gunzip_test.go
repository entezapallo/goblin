package gunzip_test

import (
	"bytes"
	"compress/gzip"
	"testing"

	"goblin/pkg/applets/gunzip"
	"goblin/pkg/core"
	"goblin/pkg/testutil"
)

func TestGunzip(t *testing.T) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, _ = zw.Write([]byte("hello\n"))
	_ = zw.Close()

	tests := []testutil.AppletTestCase{
		{
			Name:     "stdin_to_stdout",
			Args:     []string{},
			Input:    buf.String(),
			WantOut:  "hello\n",
			WantCode: core.ExitSuccess,
		},
		{
			Name:     "basic",
			Args:     []string{"input.txt.gz"},
			WantCode: core.ExitSuccess,
			Files: map[string]string{
				"input.txt.gz": buf.String(),
			},
			Check: func(t *testing.T, dir string) {
				testutil.AssertFileExists(t, dir+"/input.txt")
			},
		},
	}
	testutil.RunAppletTests(t, gunzip.Run, tests)
}
