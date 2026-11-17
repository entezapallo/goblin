package gash

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"goblin/pkg/applets/ash"
	"goblin/pkg/core"
	"goblin/pkg/gutils"
	"goblin/pkg/summon"
)

// Runner ejecuta un Script: levanta un goblin, corre el shell entre bloques
// con el ash real y envía cada grun como un paso del contrato.
type Runner struct {
	GGPath     string
	GoblinBin  string
	GuardBin   string
	RuntimeDir string
	AshBin     string // binario que entiende `--ash FILE`; si vacío, se usa sh
	Stdio      *core.Stdio
}

func (r *Runner) stdio() *core.Stdio {
	if r.Stdio != nil {
		return r.Stdio
	}
	return core.DefaultStdio()
}

func tempScript(content string) (string, error) {
	f, err := os.CreateTemp("", "gash-*.sh")
	if err != nil {
		return "", err
	}
	name := f.Name()
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(name)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	return name, nil
}

// Run corre el script y devuelve el código de salida.
func (r *Runner) Run(s *Script) int {
	stdio := r.stdio()
	name := s.Header.Name
	if name == "" {
		name = "gash-" + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}

	session, err := summon.StartSession(summon.Config{
		ID:             name,
		GGPath:         r.GGPath,
		GoblinBin:      r.GoblinBin,
		GuardBin:       r.GuardBin,
		RuntimeDir:     r.RuntimeDir,
		MemoryMB:       s.Header.MemoryMB,
		CPU:            s.Header.CPU,
		Workers:        s.Header.Workers,
		OrphanPolicy:   s.Header.Orphan,
		UnbornPolicy:   s.Header.Unborn,
		HeadlessPolicy: s.Header.Headless,
		NotifyHook:     s.Header.Notify,
	})
	if err != nil {
		fmt.Fprintf(stdio.Err, "gash: %v\n", err)
		return core.ExitFailure
	}
	defer session.Close()

	for _, item := range s.Items {
		if item.Shell != "" {
			code := ash.Run(stdio, []string{"-c", item.Shell})
			if code != core.ExitSuccess {
				return code
			}
			continue
		}
		if item.Step == nil {
			continue
		}
		def, ok := s.Defs[item.Step.Def]
		if !ok {
			fmt.Fprintf(stdio.Err, "gash: paso no definido: %s\n", item.Step.Def)
			return core.ExitFailure
		}

		bodyPath, err := tempScript(item.Step.Body)
		if err != nil {
			fmt.Fprintf(stdio.Err, "gash: %v\n", err)
			return core.ExitFailure
		}
		defer os.Remove(bodyPath)

		argv := []string{"sh", bodyPath}
		if r.AshBin != "" {
			argv = []string{r.AshBin, "--ash", bodyPath}
		}
		labels := make(map[string]string, len(def.Labels)+len(item.Step.Labels))
		for k, v := range def.Labels {
			labels[k] = v
		}
		for k, v := range item.Step.Labels {
			labels[k] = v
		}

		resp, err := session.SubmitStep(gutils.Event{
			Type:       "submit",
			Cmd:        argv,
			Labels:     labels,
			Terms:      gutils.Terms{MemoryMB: def.MemoryMB, CPUCores: def.CPU, Workers: def.Workers},
			Rules:      def.Rules,
			OnComplete: def.OnComplete,
			OnFail:     def.OnFail,
		})
		if err != nil {
			fmt.Fprintf(stdio.Err, "gash: grun %s: %v\n", item.Step.Def, err)
			return core.ExitFailure
		}
		if resp.Type == "error" {
			fmt.Fprintf(stdio.Err, "gash: grun %s: %s\n", item.Step.Def, resp.Error)
			return core.ExitFailure
		}
	}
	return core.ExitSuccess
}
