// Package summon implementa el nacimiento del goblin: crea el cgroup y el
// socket, abre el pipe de muerte, lanza cmd/goblin, hace submit del job y
// espera a que el paso cierre. Es el eslabón entre un script y un contract.
package summon

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"goblin/pkg/gutils"
	"goblin/pkg/manifest"
	"goblin/pkg/notify"
)

// Config describe el goblin a crear. Las rutas se inyectan para poder
// testear sin depender del PATH ni de la ubicación del repo.
type Config struct {
	ID             string
	Cmd            []string
	Detach         bool
	GGPath         string
	GoblinBin      string
	GuardBin       string
	RuntimeDir     string
	MemoryMB       int
	CPU            float64
	Workers        int
	OrphanPolicy   string
	UnbornPolicy   string
	HeadlessPolicy string
	NotifyHook     string
	Leftover       int
}

const (
	manifestWait = 5 * time.Second
	stepWait     = 5 * time.Minute
)

func (c *Config) fillDefaults() error {
	if c.ID == "" {
		c.ID = fmt.Sprintf("gob-%d-%d", os.Getpid(), time.Now().UnixNano())
	}
	if strings.ContainsAny(c.ID, "/.") {
		return fmt.Errorf("summon: id inválido: %s", c.ID)
	}
	if c.GGPath == "" {
		c.GGPath = "gg"
	}
	if c.GoblinBin == "" {
		c.GoblinBin = "goblin"
	}
	if c.GuardBin == "" {
		c.GuardBin = os.Getenv("GOBLIN_GUARD")
	}
	if c.GuardBin == "" {
		c.GuardBin = "gguard"
	}
	if c.RuntimeDir == "" {
		c.RuntimeDir = os.Getenv("XDG_RUNTIME_DIR")
	}
	if c.RuntimeDir == "" {
		c.RuntimeDir = os.TempDir()
	}
	return nil
}

// env arma el entorno para los procesos que lanza summon. base, si no está
// vacío, fija la base de gg para que los nombres relativos resuelvan ahí.
func (c Config) env(base string) []string {
	e := append(os.Environ(), "XDG_RUNTIME_DIR="+c.RuntimeDir)
	if base != "" {
		e = append(e, "GOBLIN_CGROUP="+base)
	}
	return e
}

func (c Config) gg(args ...string) (string, error) {
	cmd := exec.Command(c.GGPath, args...) // #nosec G204 -- rutas inyectadas por el llamador
	cmd.Env = c.env("")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (c Config) ggIn(base string, args ...string) error {
	cmd := exec.Command(c.GGPath, args...) // #nosec G204
	cmd.Env = c.env(base)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%v: %w: %s", args, err, out)
	}
	return nil
}

func (c Config) manifestPath() string {
	return filepath.Join(c.RuntimeDir, "goblin", c.ID+".json")
}

func (c Config) waitManifest() error {
	deadline := time.Now().Add(manifestWait)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(c.manifestPath()); err == nil {
			return nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("summon: el goblin no escribió su manifiesto")
}

// Session es un goblin vivo al que se le pueden mandar varios pasos.
// El pipe de muerte se mantiene abierto hasta Close().
type Session struct {
	ID         string
	Socket     string
	CGroup     string
	Base       string
	GGPath     string
	RuntimeDir string

	cfg      Config
	readEnd  *os.File
	writeEnd *os.File
	goblin   *exec.Cmd
}

// applyUnborn aplica la política cuando el goblin no llegó a nacer.
// kill (default) limpia el cgroup parcial, freeze lo congela, keep lo deja.
// Siempre deja constancia en notifications.jsonl ("log" del default).
func applyUnborn(cfg Config, ancestor string, cause error) {
	_ = notify.Emit(cfg.RuntimeDir, "unborn", cause.Error())
	policy := cfg.UnbornPolicy
	if policy == "" {
		policy = "kill"
	}
	base := filepath.Join(ancestor, "goblin")
	switch policy {
	case "keep":
		return
	case "freeze":
		fr := exec.Command(cfg.GGPath, "freeze", cfg.ID) // #nosec G204
		fr.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
		_ = fr.Run()
	default: // kill
		CleanupByName(cfg.GGPath, ancestor, cfg.ID)
	}
}

// StartSession crea el cgroup y lanza el goblin, sin hacer submit todavía.
func StartSession(cfg Config) (session *Session, err error) {
	if err := cfg.fillDefaults(); err != nil {
		return nil, err
	}
	ancestor, err := cfg.gg("discover")
	if err != nil || ancestor == "" {
		return nil, fmt.Errorf("summon: gg discover: %v", err)
	}
	// Recuperamos los cgroups que quedaron de muertes duras del goblin.
	// best-effort: si falla, el arranque sigue.
	_, _ = cfg.gg("sweep")
	base := filepath.Join(ancestor, "goblin")
	cgroup := filepath.Join(base, cfg.ID)
	created := false
	defer func() {
		if session == nil && created {
			applyUnborn(cfg, ancestor, err)
		}
	}()
	if err := cfg.ggIn(base, "create", cfg.ID); err != nil {
		return nil, fmt.Errorf("summon: %w", err)
	}
	created = true

	var limits []string
	if cfg.MemoryMB > 0 {
		limits = append(limits, fmt.Sprintf("memory=%dM", cfg.MemoryMB))
	}
	if cfg.CPU > 0 {
		limits = append(limits, fmt.Sprintf("cpu=%g", cfg.CPU))
	}
	if cfg.Workers > 0 {
		// pids.max cuenta threads en cgroup v2: un proceso Go necesita
		// muchas. Techo generoso para no matar al daemon al nacer.
		n := cfg.Workers*64 + 64
		if n < 256 {
			n = 256
		}
		limits = append(limits, fmt.Sprintf("pids=%d", n))
	}
	if len(limits) > 0 {
		if err := cfg.ggIn(base, append([]string{"limit", cfg.ID}, limits...)...); err != nil {
			return nil, fmt.Errorf("summon: %w", err)
		}
	}

	socket := filepath.Join(cfg.RuntimeDir, "goblin", cfg.ID+".sock")
	deathFD := "-1"
	var readEnd, writeEnd *os.File
	if !cfg.Detach {
		readEnd, writeEnd, err = os.Pipe()
		if err != nil {
			return nil, err
		}
		deathFD = "3"
	}

	args := []string{cfg.ID, socket, cgroup, deathFD, strconv.Itoa(os.Getpid()), cfg.GGPath}
	goblin := exec.Command(cfg.GoblinBin, args...) // #nosec G204
	goblin.Env = append(cfg.env(""),
		"GOBLIN_ORPHAN="+cfg.OrphanPolicy,
		"GOBLIN_HEADLESS="+cfg.HeadlessPolicy,
		"GOBLIN_NOTIFY="+cfg.NotifyHook,
		"GOBLIN_ON_COMPLETE="+os.Getenv("GOBLIN_ON_COMPLETE"),
		"GOBLIN_ON_FAIL="+os.Getenv("GOBLIN_ON_FAIL"),
		"GOBLIN_LEFTOVER="+strconv.Itoa(cfg.Leftover),
		"GOBLIN_GUARD="+cfg.GuardBin,
	)
	if !cfg.Detach {
		goblin.ExtraFiles = []*os.File{readEnd}
	}
	goblin.Stdout = os.Stdout
	goblin.Stderr = os.Stderr
	if err := goblin.Start(); err != nil {
		return nil, fmt.Errorf("summon: lanzar goblin: %w", err)
	}

	s := &Session{
		ID: cfg.ID, Socket: socket, CGroup: cgroup, Base: base,
		GGPath: cfg.GGPath, RuntimeDir: cfg.RuntimeDir,
		cfg: cfg, readEnd: readEnd, writeEnd: writeEnd, goblin: goblin,
	}
	// El pipe lo abre el hijo, así que el padre puede soltar su copia.
	if readEnd != nil {
		defer readEnd.Close()
		s.readEnd = nil
	}
	if err := cfg.waitManifest(); err != nil {
		s.release()
		return nil, err
	}
	return s, nil
}

// Submit manda un evento al goblin de la sesión.
func (s *Session) Submit(e gutils.Event) (gutils.Event, error) {
	return gutils.Call(s.Socket, e)
}

// SubmitStep hace submit y espera a que el paso cierre.
func (s *Session) SubmitStep(e gutils.Event) (gutils.Event, error) {
	resp, err := s.Submit(e)
	if err != nil || resp.Type != "submit" {
		return resp, err
	}
	if err := s.WaitClosed(resp.Step); err != nil {
		return resp, err
	}
	return resp, nil
}

// WaitClosed espera a que el paso figure closed en el resumen del actor.
func (s *Session) WaitClosed(step string) error {
	deadline := time.Now().Add(stepWait)
	for time.Now().Before(deadline) {
		resp, err := gutils.Call(s.Socket, gutils.Event{Type: "list"})
		if err == nil {
			for _, info := range resp.Steps {
				if info.ID == step && info.State == "closed" {
					return nil
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("summon: el paso %s no cerró", step)
}

// release cierra el pipe: el goblin ve EOF y aplica on_orphan.
func (s *Session) release() {
	if s.writeEnd != nil {
		_ = s.writeEnd.Close()
		s.writeEnd = nil
	}
}

// Close retira la sesión y limpia el cgroup.
func (s *Session) Close() {
	s.release()
	if s.goblin != nil {
		_ = s.goblin.Wait()
	}
	CleanupByName(s.GGPath, filepath.Dir(s.Base), s.ID)
}

// Launch es el atajo de un solo job: crea la sesión, hace submit y espera
// el cierre del paso (o devuelve apenas en --detach).
func Launch(cfg Config) (string, error) {
	if len(cfg.Cmd) == 0 {
		return "", fmt.Errorf("summon: falta el comando a lanzar")
	}
	s, err := StartSession(cfg)
	if err != nil {
		return "", err
	}
	// En modo normal cerramos la sesión completa: el goblin ya se retiró al
	// ver EOF del pipe, así que Wait() termina y se limpia su cgroup. En
	// --detach no se cierra: el goblin debe sobrevivir a summon.
	if !cfg.Detach {
		defer s.Close()
	}

	resp, err := s.Submit(gutils.Event{
		Type:  "submit",
		Cmd:   cfg.Cmd,
		Terms: gutils.Terms{MemoryMB: cfg.MemoryMB, CPUCores: cfg.CPU, Workers: cfg.Workers},
	})
	if err != nil {
		return "", fmt.Errorf("summon: submit: %w", err)
	}
	if resp.Type == "error" {
		return "", fmt.Errorf("summon: submit: %s", resp.Error)
	}
	if !cfg.Detach && resp.Step != "" {
		if err := s.WaitClosed(resp.Step); err != nil {
			return "", err
		}
	}
	return cfg.ID, nil
}

// CleanupByName borra el cgroup de un goblin ya terminado (best-effort).
func CleanupByName(ggPath, ancestor, id string) {
	base := filepath.Join(ancestor, "goblin")
	kill := exec.Command(ggPath, "kill", id) // #nosec G204
	kill.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
	_ = kill.Run()
	remove := exec.Command(ggPath, "remove", id) // #nosec G204
	remove.Env = append(os.Environ(), "GOBLIN_CGROUP="+base)
	_ = remove.Run()
}

// ReadManifest lee directamente la placa (sin depender de XDG_RUNTIME_DIR).
func ReadManifest(runtimeDir, id string) (*manifest.Manifest, error) {
	data, err := os.ReadFile(filepath.Join(runtimeDir, "goblin", id+".json"))
	if err != nil {
		return nil, err
	}
	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
