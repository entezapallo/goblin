<table>
  <tr>
    <td width="190" valign="top" align="center">
      <img src=".media/logo.jpeg" alt="Logo de Bergënot" width="160" />
      <br />
      <sub>Logo de <strong>Bergënot</strong>, grupo de música.</sub>
    </td>
    <td valign="top">
      <h1>goblin</h1>
      <p><em>Una abstracción de scripts con semántica de <strong>contracts de illumos</strong>, construida en estilo Unix sobre <strong>cgroups v2</strong>: shell POSIX + un puñado de applets chiquitos, propios y compatibles, que dan a cada script un subárbol de procesos con límites, pausa, muerte confiable y eventos observables.</em></p>
      <p><code>goblin</code> es un hard fork podado de <a href="https://github.com/rcarmo/go-busybox">rcarmo/go-busybox</a> (MIT). El fork aporta el <strong>mecanismo</strong> — un <code>ash</code> POSIX real y probado, parsing de <code>/proc</code>, applets de señales y el stdlib de scripting. <code>goblin</code> aporta la <strong>política</strong>: qué significa cada estado, evento y señal en términos de un contrato de ciclo de vida.</p>
    </td>
  </tr>
</table>

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
| `go test ./...` | ✅ Verde: 45 paquetes OK. El test diferencial de `ash` se salta con motivo explícito (faltan sus fixtures externos). |
| `go vet ./...` | ✅ Pasa. |
| Motor | ✅ Fase 4: `cmd/goblin` es el daemon efímero; `gutils` es la política sobre cgroupfs; `cmd/grimoire` reemplaza al dispatcher viejo. `homunculi` y `setpgid` desaparecidos. |
| Capa de aislamiento | ✅ `scripts/gg`: superset de cgroups v2 en shell POSIX, agnóstico a init; `make test-gg` y `TestGoblinLifecycle` pasan contra cgroupfs real. |
| Direccionamiento | ✅ Manifiestos JSON (`pkg/manifest`), `pkg/discover` resuelve `-g` → `GOBLIN_ID` → ancestría. |
| Protocolo | ✅ Híbrido: consultas leen cgroupfs; mutaciones (`submit`/`pause`/`resume`/`kill`) van por socket NDJSON al goblin. |
| Contrato | ✅ Fase 5: `pkg/contract` con estados goblin→paso→worker, reglas `fail_when`/`abort_when`/`notify_when`, hooks y hechos reales (`memory.peak`, `oom_kill`, salida de texto, tiempo). |
| Restos de WASM | ✅ Eliminados: `pkg/sandbox/` y los 9 `*_wasm.go`. `pkg/core/fs` y `awk` dejaron de depender del sandbox. |
| Documentación | Este README reemplaza al de go-busybox (57 applets, TinyGo, 387/387). |
| Applets huérfanos | ✅ Conectados: `cut`, `diff`, `grep`, `printf`, `sed`, `sort`, `tr`, `uniq` están en el registry (51 entradas) y tienen `cmd/<applet>` propio. |
| Git | Commit inicial `a45d358` (*last commit antes q refactor grande*). La limpieza de la fase 1 está sin commitear. Remote: `git@github-entezapallo:entezapallo/goblin.git`. |
| `dev/` | Repo git anidado vacío (solo `.git/`, sin commits). Ignorado vía `.gitignore`; no participa del proyecto. |

### Cómo se alcanzó el baseline verde

| Paquete | Arreglo |
|---|---|
| `pkg/applets/ash` | `testutil.RunAppletTests` ahora hace snapshot/restauración del entorno por subtest y tiene un campo `Env` opt-in; `export_p_builtin` pide entorno vacío. Los casos `assignment` (semántica POSIX de expansiones) y `unalias_builtin` (127 = *command not found*) tenían expectativas incorrectas. |
| `pkg/applets/ash` | `TestAshBusyboxDiff` se salta cuando no están el fixture `.config` y el binario `busybox-reference`, que se podaron a propósito. Es un test diferencial contra un binario externo; no puede correr sin él. |
| `pkg/applets/awk` | `strftime` no existe en goawk (es extensión de gawk): se registró como función nativa y se unificó la implementación en `formatStrftime`. |
| `pkg/applets/time` | El test buscaba `real` en stdout; `time` escribe el resumen a stderr. Se corrigió el test, no el applet. |

Con esto queda una línea base limpia: cualquier fallo nuevo a partir de ahora es una regresión del refactor.

---

## 3. Proyecto actual (estructura real)

```
goblin/
├── cmd/                     # 47 entry points
│   ├── goblin/              # daemon efímero del contrato (nuevo)
│   ├── grimoire/            # dispatcher multi-call (51 applets, antes cmd/goblin)
│   └── <applet>/            # ash, awk, cat, cp, cut, diff, echo, find, grep, ...
├── pkg/
│   ├── applets/             # 53 paquetes (todos los del registry + helpers)
│   │   ├── ash/             # intérprete POSIX heredado (~11.900 líneas, 349/349)
│   │   ├── procutil/        # señales y matching de /proc (del fork)
│   │   ├── gprocutil/       # ReadStat/Children/Descendants
│   │   ├── startstopdaemon/ # + SpawnInner (UseCgroupFD + Pdeathsig)
│   │   ├── pstree/          # árbol de procesos
│   │   └── goblin*/         # 5 verbos: resuelven target y mutan por socket
│   │       goblinsubmit, goblinpause, goblinresume, goblinkill, goblinlist
│   ├── core/                # fs, archiveutil, textutil, fileutil, timeutil
│   ├── contract/            # lógica pura: estados, hechos, reglas, hooks, Effects
│   ├── gutils/              # actor que aplica Effects sobre cgroupfs + NDJSON + SpawnInto
│   ├── manifest/            # placas <id>.json (directorio de goblins vivos)
│   ├── discover/            # listar placas y resolver -g / GOBLIN_ID / ancestría
│   ├── verbs/               # ParseRef + Mutate, compartidos por los 5 verbos
│   ├── history/             # append de history.jsonl + Compact
│   ├── integration/, testutil/, testdata/
├── scripts/
│   ├── gg                   # superset de cgroups v2 (shell POSIX, agnóstico a init)
│   └── gg_test.sh           # prueba end-to-end contra cgroupfs real
├── mds/                     # diseño target (ver §6)
├── docs/SPEC.md             # especificación heredada del fork
├── HANDOFF.md, plan.md      # registros históricos del diseño v1
├── Makefile                 # mínimo: build, vet, sizes, clean
├── go.mod                   # module goblin, go 1.25.6
└── README.md                # este archivo
```

Los artefactos sueltos de la raíz y los scripts one-off (`cleanup.sh`, `info.bash`, `structure.txt`, `greeting`) se borraron en la fase 1. `scripts/` volvió a crearse en la fase 3, ahora con `gg` y su prueba.

---

## 4. Mapa de verbos (implementado en la fase 4)

Los verbos conservan su interfaz; cambió el motor. Modelo **híbrido**: las consultas leen cgroupfs/manifiestos directo; las mutaciones van por socket NDJSON al goblin para que la política medie cada acción.

| Verbo | Ruta | Implementación |
|---|---|---|
| `submit` | socket | `verbs.Mutate` → actor → `gg create <id>/w-N` + `SpawnInto` (`UseCgroupFD` + `Pdeathsig`) |
| `pause` | socket | actor → `gg freeze <id>/w-N` → `cgroup.freeze = 1` |
| `resume` | socket | actor → `gg thaw <id>/w-N` → `cgroup.freeze = 0` |
| `kill` | socket | actor → `gg kill <id>/w-N` → `cgroup.kill` del subárbol |
| `list` | directo | `discover.Discover()` lee manifiestos y verifica que el cgroup exista |
| `death` (interno) | pipe | `watchDeathPipe` ve EOF → actor hace `gg kill` + `gg remove` de cada worker y borra el manifiesto |

El direccionamiento es `-g <ref>` → manifiesto → `GOBLIN_ID` → ancestría por `/proc` → error con candidatos. El goblin es un **daemon efímero**: no persiste; cuando muere, su manifiesto desaparece y sus workers ya fueron matados. No hay `warlock` autómata.

### El contrato (`pkg/contract`)

La semántica es lógica pura y no depende de cgroupfs; devuelve `Effect`s que el actor aplica:

- **Estados:** goblin `alive → running → (orphaned) → dying → dead`; paso `pending → starting → active → draining → closing → closed{success|failed|aborted}`; worker `spawning → running → exited{killed|oom_killed}`.
- **Hechos:** `oom`, `mem_over`, `mempeak` (de `memory.peak`), `t`, `failed_count`, coincidencias de texto en stdout/stderr (evaluadas con `regexp`/`strings` nativos, sin applets).
- **Reglas:** `fail_when` (falla al cerrar), `abort_when` (fail-fast, mata a los demás, once por paso), `notify_when` (con enfriamiento de 1 minuto). Constructores: `TextRule`, `OOMRule`, `FailedCountRule`, `TimeoutRule`, `MemOverRule`.
- **Hooks:** `on_complete` / `on_fail` al cerrar el paso, `on_orphan` (keep/kill/freeze) al morir el instructor, y `on_leftover` (SIGTERM → espera → kill del cgroup del paso).
- **Garantía dura:** al morir el goblin, sus workers están en su cgroup y se matan con `cgroup.kill`; el manifiesto desaparece con él.

Las reglas se inyectan en `gutils.Config.Rules` (no serializables a NDJSON: son funciones), y `cmd/goblin` las recibirá de `gash` cuando la DSL pase el contrato. Las variables de entorno `GOBLIN_ORPHAN`, `GOBLIN_NOTIFY`, `GOBLIN_ON_COMPLETE`, `GOBLIN_ON_FAIL` y `GOBLIN_LEFTOVER` configuran las políticas del daemon.

### Comandos de `gg` (implementados en `scripts/gg`)

- `gg discover` — desde `/proc/mounts` y `/proc/self/cgroup`, camina hacia arriba hasta el primer ancestro **escribible y sin procesos internos** (regla *no internal process* de cgroup v2). No mira `user.slice`, no llama a `systemctl`.
- `gg create NAME` — crea el subgrupo y habilita en `cgroup.subtree_control` del padre los controladores disponibles (`cpu`, `memory`, `pids`, `io`, `cpuset`).
- `gg enter NAME PID` — mueve un proceso real al grupo.
- `gg limit NAME memory=2G cpu=1 pids=4` — escribe `memory.max`, `cpu.max` (quota/period) y `pids.max`; acepta sufijos K/M/G.
- `gg freeze|thaw NAME` — `cgroup.freeze` = 1/0 (pausa real, no `SIGSTOP`).
- `gg kill NAME` — `cgroup.kill` del subárbol, con fallback a freeze + kill por PID en kernels < 5.14.
- `gg events NAME [FILE]` — stream de cambios en `cgroup.events` (o `memory.events`) por polling de 1s.
- `gg wait NAME`, `gg stat NAME`, `gg list`, `gg remove NAME`.

`GOBLIN_CGROUP` sobreescribe la base para tests. `make test-gg` corre `scripts/gg_test.sh`: crea una base temporal, verifica create/limit/enter/freeze/thaw/kill/wait/remove contra cgroupfs real y limpia todo al salir.

---

## 5. Plan de migración

Cada fase termina con `go build ./...` verde y, desde la fase 2, `go test ./...` verde. Nunca borrar dos piezas sin compilar en el medio.

0. **Checkpoint** (hecho): commit `a45d358` antes del refactor.
1. **Higiene sin cambiar comportamiento** (hecho): borrados `pkg/sandbox/`, los 9 `*_wasm.go` y los artefactos sueltos; limpio código muerto; `gofmt` en todo el árbol; conectados los 8 applets huérfanos (registry de 51 entradas). Sin regresiones: mismos 4 fallos preexistentes, `go build` verde.
2. **Baseline verde** (hecho): `testutil.go` aísla/restaura el entorno por subtest; expectativas de `ash` corregidas; `strftime` registrado en goawk; `time` verifica stderr. `go test ./...` pasa.
3. **Núcleo nuevo antes de sacar el viejo** (hecho): `scripts/gg` implementa *discover/create/enter/limit/freeze/thaw/kill/events/wait/stat/list/remove*; `make test-gg` demuestra create→enter→freeze→kill→remove con un proceso real.
4. **Retirar el motor v1 y reasentar verbos** (hecho): manifiestos + `discover` por id; 5 verbos reescritos (híbridos); actor sobre `gg` y `SpawnInner`; `cmd/goblin` como daemon efímero, `cmd/grimoire` como dispatcher; `homunculi` y `setpgid` eliminados. `TestGoblinLifecycle` cubre submit→pause→resume→kill→muerte del instructor.
5. **El contrato** (hecho): `pkg/contract` separa la semántica (estados goblin→paso→worker, hechos, reglas, hooks) del actor que aplica Effects. `TestContractRulesEndToEnd` prueba un `abort_when` sobre texto que mata el worker y dispara `on_fail` con cgroupfs real.
6. **`gash`** (pendiente): extender `ash` con `grun…nurg`, `def…let…tel`, `resources`, `notify` y gramática de condiciones; antes, investigar los dos puntos de dispatch (~1048 y ~2749). Los nombres se fijan en esa sesión.
7. **Fachada y superficies** (pendiente): README/Makefile definitivos, archivar handoffs, `gprocutil` con `Ancestors`/`Subtree` y campo `starttime`; `orb`/`raven`/`gobblr` diferidos (el goblin es el daemon efímero, no hay `warlock`).

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
go test ./...           # unit tests (45 paquetes verdes)
go vet ./...
make build              # atajo del Makefile
make test-gg            # prueba end-to-end de ggroups contra cgroupfs real
go test ./pkg/contract -v                          # estados, reglas, hooks (lógica pura)
go test ./pkg/gutils -v                            # daemon + abort_when real sobre cgroupfs
./scripts/gg discover                              # ver el ancestro cgroup2 que usa el sistema
# binarios: cmd/goblin (daemon efímero), cmd/grimoire (applets)
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
