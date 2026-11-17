# goblin

> Una abstracción de scripts con semántica de **contracts de illumos**, construida en estilo Unix sobre **cgroups v2**: shell POSIX + un puñado de applets chiquitos, propios y compatibles, que dan a cada script un subárbol de procesos con límites, pausa, muerte confiable y eventos observables.

`goblin` es un hard fork podado de [`rcarmo/go-busybox`](https://github.com/rcarmo/go-busybox) (MIT). El fork aporta el **mecanismo** — un `ash` POSIX real y probado, parsing de `/proc`, applets de señales y el stdlib de scripting. `goblin` aporta la **política**: qué significa cada estado, evento y señal en términos de un contrato de ciclo de vida.

---

## 1. Objetivo

Tomar el fork de `go-busybox` y convertirlo en un subsistema donde **un script pueda declarar un contract**: un conjunto de procesos con identidad, límites de recurso, estados observables y una garantía de muerte. La frontera de contención es el unified hierarchy de cgroups v2, que es la abstracción de procesos bien hecha del kernel; **no** el hierarchy de una distro en particular.

En forma esquemática:

```
un script .gsh  ──▶  ggroups (cgroups v2)  ──▶  procesos
                       │
                       ├── límites: memory.max / cpu.max / pids.max
                       ├── pausa:   cgroup.freeze
                       ├── muerte:  cgroup.kill  (sin huérfanos)
                       └── eventos: cgroup.events / memory.events
```

### Garantías (distintas, no confundir)

1. **Goblin → workers (dura, sin excepciones):** si el goblin muere, muere todo su subárbol. Se logra con `cgroup.kill` más un pipe de muerte heredado del script (EOF cuando el instructor muere, incluso por `SIGKILL`).
2. **Instructor (`gash`, R, terminal) → goblin (blanda, configurable):** el goblin está diseñado para sobrevivir al instructor. Si este muere, aplica `on_orphan` (default: `keep` + notificar).

### Decisiones cerradas

| Decisión | Conclusión |
|---|---|
| ¿Qué es el fork? | La caja de herramientas, no el producto. Se poda y se extiende con utilidades mínimas propósito-específico. |
| ¿Dónde vive el estado? | En cgroupfs (más un directorio plano de manifiestos/historial). No hay daemon que posea el mapa de workers. |
| ¿Qué es `ggroups`? | El núcleo: un superset de cgroups v2 en shell + applets, interfaz texto/pipes, corte Unix. |
| ¿Cómo se descubre cgroup2? | Leyendo `/proc/mounts`: el ancestro escribible más cercano. Sin `systemctl`, sin `user.slice`, sin asumir init. |
| ¿Cómo se modela un contract? | Jerarquía cgroup (goblin → paso → worker) + pipe de muerte + eventos de cgroup. |
| ¿Y los recursos? | Nativos de cgroups: `memory=2G → memory.max`, `cpu=1 → cpu.max`, `workers → pids.max`. `ulimit` queda como herramienta del usuario, no como mecanismo del sistema. |
| ¿Y los verbos? | **Se quedan.** Cambia su implementación, no su interfaz: pasan de hablar NDJSON a un actor Go a operar directamente sobre cgroupfs. |

### Lo que deliberadamente **no** es

- No es un reemplazo de systemd, ni un init, ni un scheduler de clúster.
- No es un daemon Go de larga vida con un mapa en memoria (ese fue el diseño v1 y se retira).
- No depende de cgroups *de systemd*: se usa la abstracción del kernel, no la del distro.

---

## 2. Punto actual (diagnóstico verificado)

Estado del árbol tal como está hoy:

| Ítem | Estado |
|---|---|
| `go build ./...` | ✅ Pasa. |
| `go test ./...` | ❌ Falla en 4 paquetes (detalle abajo). |
| Motor v1 | Vivo: `pkg/gutils` (actor + protocolo NDJSON), `cmd/homunculi`, 5 applets `goblin*`. |
| Capa de aislamiento | **Aún no implementada**: no existe `ggroups`. Hoy se usa `setpgid` + death-pipe. |
| Restos de WASM | ✅ Eliminados: `pkg/sandbox/` y los 9 `*_wasm.go`. `pkg/core/fs` y `awk` dejaron de depender del sandbox. |
| Documentación | Este README reemplaza al de go-busybox (57 applets, TinyGo, 387/387). |
| Applets huérfanos | ✅ Conectados: `cut`, `diff`, `grep`, `printf`, `sed`, `sort`, `tr`, `uniq` están en el registry (51 entradas) y tienen `cmd/<applet>` propio. |
| Git | Commit inicial `a45d358` (*last commit antes q refactor grande*). La limpieza de la fase 1 está sin commitear. Remote: `git@github-entezapallo:entezapallo/goblin.git`. |
| `dev/` | Repo git anidado vacío (solo `.git/`, sin commits). Ignorado vía `.gitignore`; no participa del proyecto. |

### Fallos de test conocidos (preexistentes al refactor)

| Paquete | Test | Causa |
|---|---|---|
| `pkg/applets/ash` | `TestAshBusyboxDiff` | Falta el fixture `testdata/ash_test/.config` (podado del árbol). |
| `pkg/applets/ash` | `TestAsh` | `testutil.go:200` no aísla el entorno del proceso hijo; los casos `assignment`, `unalias_builtin` y `export_p_builtin` ven variables del host. |
| `pkg/applets/awk` | `TestStrftime` | Sale con código 1 y sin output en `epoch`, `day_of_year`, `weekday_u_w` (sospecha de `LC_TIME`/zona horaria). |
| `pkg/applets/time` | `TestTime` | La salida no contiene `real` (sospecha de captura de stderr en el harness). |

Estos se arreglan **antes** de la migración: sin baseline verde no se puede distinguir una regresión nueva de un fallo viejo.

---

## 3. Proyecto actual (estructura real)

```
goblin/
├── cmd/                     # 47 entry points
│   ├── goblin/              # dispatcher multi-call (51 applets en su registry)
│   ├── homunculi/           # servidor NDJSON + death-pipe del diseño v1
│   └── <applet>/            # ash, awk, cat, cp, cut, diff, echo, find, grep, ...
├── pkg/
│   ├── applets/             # 53 paquetes (todos los del registry + helpers)
│   │   ├── ash/             # intérprete POSIX heredado (~11.900 líneas, 349/349)
│   │   ├── procutil/        # señales y matching de /proc (del fork)
│   │   ├── gprocutil/       # ReadStat/Children/Descendants (nuevo)
│   │   ├── pstree/          # árbol de procesos (nuevo)
│   │   └── goblin*/         # 5 clientes NDJSON del diseño v1
│   │       goblinsubmit, goblinpause, goblinresume, goblinkill, goblinlist
│   ├── core/                # fs, archiveutil, textutil, fileutil, timeutil
│   ├── gutils/              # motor v1: State/Terms/Event, Call/Serve, actor, Spawn(setpgid)
│   ├── discover/            # enumera sockets por PID en $XDG_RUNTIME_DIR/goblin
│   ├── history/             # append de history.jsonl + Compact
│   ├── integration/, testutil/, testdata/
├── mds/                     # diseño target (ver §6)
├── docs/SPEC.md             # especificación heredada del fork
├── HANDOFF.md, plan.md      # registros históricos del diseño v1
├── Makefile                 # mínimo: build, vet, sizes, clean
├── go.mod                   # module goblin, go 1.25.6
└── README.md                # este archivo
```

Los artefactos sueltos de la raíz y los scripts one-off (`cleanup.sh`, `scripts/`, `info.bash`, `structure.txt`, `greeting`) se borraron en la fase 1 y se agregaron a `.gitignore`.

---

## 4. Hacia dónde va: mapa de verbos

Los verbos **no desaparecen**: se les cambia el motor. La interfaz (nombre, texto de entrada/salida, código de exit) se mantiene estable.

| Verbo | Implementación actual (v1) | Implementación target (cgroupfs) |
|---|---|---|
| `submit` | `gutils.Call(sock, Event{Type:"submit", Cmd})` al actor | `gg create <id>/step-N/w-i` + `SpawnInner` con `CgroupFD`/`Pdeathsig`; escribe el manifiesto |
| `pause` | `syscall.Kill(-pgid, SIGSTOP)` | escribir `1` en `cgroup.freeze` |
| `resume` | `syscall.Kill(-pgid, SIGCONT)` | escribir `0` en `cgroup.freeze` |
| `kill` | `killTree` = `Kill(-pgid)` + `Descendants()` | `cgroup.kill` del subárbol (fallback en kernels < 5.14) |
| `list` | NDJSON `list`, resultado colado en `Event.Error` | enumerar manifiestos + `cgroup.procs`, JSON por línea |
| `death` (interno) | goroutine `watchDeathPipe` en Go | watchdog en shell sobre el fd heredado → `gg kill` en cascada |

Lo que sí se retira: `Serve`/`Call`, el socket NDJSON, el mapa `workers` del actor y `homunculi` como servidor Go. El único pedazo compilado que se conserva es `startstopdaemon.SpawnInner` (`UseCgroupFD` + `Pdeathsig`), porque el shell no puede hacer eso.

### Comandos previstos para `ggroups`

- `gg discover` — encontrar el ancestro de cgroup2 escribible desde `/proc/mounts`; fallar con mensaje claro si no hay.
- `gg create <name>` / `gg enter <name>` — crear subgrupos; habilitar controladores en `cgroup.subtree_control` del padre.
- `gg limit <name> memory=… cpu=… pids=…` — escribir los `*.max`.
- `gg freeze|thaw <name>`, `gg kill <name>`, `gg events|wait <name>`, `gg stat <name>`.

---

## 5. Plan de migración

Cada fase termina con `go build ./...` verde y, desde la fase 2, `go test ./...` verde. Nunca borrar dos piezas sin compilar en el medio.

0. **Checkpoint** (hecho): commit `a45d358` antes del refactor.
1. **Higiene sin cambiar comportamiento** (hecho): borrados `pkg/sandbox/`, los 9 `*_wasm.go` y los artefactos sueltos; limpio código muerto; `gofmt` en todo el árbol; conectados los 8 applets huérfanos (registry de 51 entradas). Sin regresiones: mismos 4 fallos preexistentes, `go build` verde.
2. **Baseline verde** (pendiente): arreglar `testutil.go:200`, recuperar/skippear el fixture `.config`, e investigar `TestStrftime` y `TestTime`.
3. **Núcleo nuevo antes de sacar el viejo:** implementar `gg` (*discover/create/limit/kill/freeze/events/stat*) y demostrar la garantía dura con un job de juguete.
4. **Retirar el motor v1:** borrar `gutils`, el socket y `homunculi`; **reescribir los 5 verbos** sobre cgroupfs; `discover` pasa a leer manifiestos; renombrar `cmd/goblin` → `cmd/grimoire` para liberar el nombre.
5. **Reasentar el fork:** applets mecanismo y stdlib; único Go nuevo justificado, `SpawnInner`; `gprocutil` gana `Ancestors`/`Subtree` **y el campo `starttime`** (hoy `ReadStat` no lo expone, aunque los docs lo asumen).
6. **Fachada:** README y Makefile definitivos, un único doc de objetivo en `mds/`, handoffs archivados, y resolución de los nombres pendientes (`gsh`→`gash`, `run...nur`→`grun...nurg`, `def` vs `define`).

---

## 6. Documentos

| Archivo | Rol |
|---|---|
| `mds/structure-spec.md` | Organización: binarios, paquetes y migración pieza por pieza (§4). |
| `mds/goblin_mechanism.md` | Mecánica del goblin: subárbol, instructor, máquina de estados, reglas, applets. |
| `mds/ggroups-spec.md` | Capa cgroups v2 (kernel/cgroups). |
| `mds/wire-spec.md`, `warlock-spec.md`, `orb-spec.md` | Protocolo, autómata y TUI. |
| `mds/gash-handoff-v2.md` | DSL `gash`: `grun…nurg`, `def…let…tel`, `resources`, `notify`, condiciones; preguntas abiertas. |
| `HANDOFF.md`, `plan.md` | Diseño v1 y log de 21 fixes de `ash`; registro histórico, **no** fuente de verdad. |

---

## 7. Cómo trabajar

```bash
go build ./...          # compilar todo
go test ./...           # unit tests (ver fallos conocidos en §2)
go vet ./...
make build              # atajo del Makefile
```

Estado del remoto y publicación:

```bash
git remote -v                          # git@github-entezapallo:entezapallo/goblin.git
ssh -T github-entezapallo              # debe decir "Hi entezapallo!"
git push -u origin dev
```

Si `ssh -T` identifica a otra cuenta (`jcarocont`), la llave `~/.ssh/id_ed25519-gitlab` no corresponde a `entezapallo` y hay que registrar la llave correcta en esa cuenta de GitHub.

---

## Licencia

MIT (heredada del fork de `go-busybox`).
