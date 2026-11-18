# goblin <img src=".media/logo.jpeg" alt="Logo de Bergënot" align="right" height="138" />
> a lesser daemon


Un subsistema Unix de ejecución de scripts con semántica de **contracts de illumos**, construido en estilo Unix sobre **cgroups v2**: shell POSIX + applets chiquitos que dan a cada job un subárbol con límites, pausa, muerte confiable y eventos observables. El logo es de **Bergënot**.

`goblin` es un hard fork de [`rcarmo/go-busybox`](https://github.com/rcarmo/go-busybox) (MIT). El fork aporta el mecanismo (un `ash` POSIX probado, `/proc`, applets de señales); `goblin` aporta la política: qué significa cada estado, evento y señal en términos de un contrato.

## Garantías

1. **Dura:** si el goblin muere, mueren todos sus workers — sin huérfanos a nivel de SO. Lo da `gguard` con un pipe de liveness, no `Pdeathsig`.
2. **Blanda:** el goblin sobrevive a su instructor (`gash`, R, terminal) y aplica `on_orphan` (default `keep` + notificar).

## Quickstart

```bash
make build          # compila los binarios en _build/
make install        # los instala en ~/.local/bin + symlinks de grimoire

# un job suelto
summon -- sh -c 'echo hola'

# un script con contract
cat > pipeline.gsh <<'EOF'
name demo
resources memory=512 cpu=1.5 workers=2

define alinear
let
    memory=256
    cpu=1
    abort_when {match 'CUDA out of memory' in stderr}
    on_job_exit echo listo
tel

grun alinear part=1
    echo trabajando
    sleep 1
nurg
EOF
gash pipeline.gsh

# gestionar en vivo
orb                 # TUI: j/k, p/r/x, ?, q
```

## Binarios

| Binario | Rol |
|---|---|
| `gash` | Compila y corre scripts `.gsh` (DSL sobre `ash`) |
| `summon` | Crea el goblin, entrega el job y espera |
| `goblin` | Daemon efímero del contrato (uno por script) |
| `gguard` | Supervisor de liveness por worker |
| `raven` | Emisor de notificaciones (`notifications.jsonl`) |
| `orb` | TUI estilo `btm` para observar y actuar |
| `grimoire` | Los applets del fork (multi-call) |

También está `scripts/gg`, el superset de cgroups v2 (`discover`, `create`, `limit`, `freeze`, `kill`, `sweep`, …).

## Documentación

- [Getting started](docs/getting-started.md) — instalación, requisitos y primer job.
- [La DSL `gash`](docs/gash.md) — sintaxis completa de `.gsh` y reglas.
- [Lógica](docs/logica.md) — modelo mental, estados, garantías y decisiones.
- [Tools](docs/tools.md) — los binarios y `gg` uno por uno.
- [Using](docs/using.md) — casos de uso, observabilidad y troubleshooting.
- [Migration](docs/MIGRATION.md) — bitácora del refactor desde go-busybox.

## Verificación

```bash
go test ./...       # 51 paquetes
make test-gg        # prueba end-to-end contra cgroupfs real
```

Requiere Linux con cgroup v2 delegado al usuario (`cpu memory pids` en `cgroup.subtree_control`).

## Licencia

MIT (heredada del fork de `go-busybox`).
