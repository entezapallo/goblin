# goblin <img src=".media/logo.jpeg" alt="Logo de Bergënot" align="right" height="138" />
> a lesser daemon

Una abstracción de scripts con semántica de **contracts de illumos**, construida en estilo Unix sobre **cgroups v2**: shell POSIX + un puñado de applets chiquitos, propios y compatibles, que dan a cada script un subárbol de procesos con límites, pausa, muerte confiable y eventos observables.

`goblin` es un hard fork podado de [`rcarmo/go-busybox`](https://github.com/rcarmo/go-busybox) (MIT). El fork aporta el **mecanismo** — un `ash` POSIX real y probado, parsing de `/proc`, applets de señales y el stdlib de scripting. `goblin` aporta la **política**: qué significa cada estado, evento y señal en términos de un contrato de ciclo de vida. El logo es de **Bergënot**, grupo de música.



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
| `go test ./...` | ✅ Verde: 47 paquetes OK. El test diferencial de `ash` se salta con motivo explícito (faltan sus fixtures externos). |
| `go vet ./...` | ✅ Pasa. |
| Motor | ✅ Fase 4: `cmd/goblin` es el daemon efímero; `gutils` es la política sobre cgroupfs; `cmd/grimoire` reemplaza al dispatcher viejo. `homunculi` y `setpgid` desaparecidos. |
| Capa de aislamiento | ✅ `scripts/gg`: superset de cgroups v2 en shell POSIX, agnóstico a init; `make test-gg` y `TestGoblinLifecycle` pasan contra cgroupfs real. |
| Direccionamiento | ✅ Manifiestos JSON (`pkg/manifest`), `pkg/discover` resuelve `-g` → `GOBLIN_ID` → ancestría. |
| Protocolo | ✅ Híbrido: consultas leen cgroupfs; mutaciones (`submit`/`pause`/`resume`/`kill`) van por socket NDJSON al goblin. |
| Contrato | ✅ Fase 5: `pkg/contract` con estados goblin→paso→worker, reglas `fail_when`/`abort_when`/`notify_when`, hooks y hechos reales (`memory.peak`, `oom_kill`, salida de texto, tiempo). |
| Nacimiento | ✅ `summon` (`cmd/summon` + `pkg/summon`): crea cgroup/socket/pipe, lanza el goblin, hace submit y espera el cierre del paso. Hay test end-to-end en modo normal y `--detach`. |
| DSL | ✅ Fase 7: `gash` (`cmd/gash` + `pkg/gash`) parsea `.gsh` con header, `define…let…tel` y `grun…nurg`; los cuerpos corren en el `ash` real y cada `grun` es un paso del contrato. |
| Robustez | ✅ Garantía dura con `gguard` + pipe de liveness; `Call` con timeout; `Launch` limpia su cgroup; `gg sweep` recupera huérfanos. |
| Observabilidad | ✅ `notify_when` escribe en `notifications.jsonl` vía `pkg/notify`/`raven`; labels en hooks; `history.jsonl`; struct de fallo con `stderr_tail` y `status.json`. |
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
│   ├── goblin/              # daemon efímero del contrato
│   ├── gguard/              # supervisor de liveness por worker (fd 3 → EOF → SIGKILL)
│   ├── raven/               # emisor de notificaciones (CLI sobre pkg/notify)
│   ├── orb/                 # TUI: tablas + sparklines + eventos, layout configurable
│   ├── summon/              # launcher: cgroup + socket + pipe + submit + espera
│   ├── gash/                # frontend de la DSL .gsh sobre ash
│   ├── grimoire/            # dispatcher multi-call (51 applets, antes cmd/goblin)
│   └── <applet>/            # ash, awk, cat, cp, cut, diff, echo, find, grep, ...
├── pkg/
│   ├── applets/             # 53 paquetes (todos los del registry + helpers)
│   │   ├── ash/             # intérprete POSIX heredado (~11.900 líneas, 349/349)
│   │   ├── procutil/        # señales y matching de /proc (del fork)
│   │   ├── gprocutil/       # Identity/Ancestors/Subtree/Children/Descendants
│   │   ├── startstopdaemon/ # + SpawnInner (UseCgroupFD + ExtraFiles)
│   │   ├── pstree/          # árbol de procesos
│   │   └── goblin*/         # 5 verbos: resuelven target y mutan por socket
│   │       goblinsubmit, goblinpause, goblinresume, goblinkill, goblinlist
│   ├── core/                # fs, archiveutil, textutil, fileutil, timeutil
│   ├── contract/            # estados, hechos, reglas, hooks, Effects y RuleSpec
│   ├── gash/                # parser de .gsh + runner (prepass sobre ash)
│   ├── gutils/              # actor que aplica Effects sobre cgroupfs + NDJSON + SpawnInto
│   ├── manifest/            # placas <id>.json (directorio de goblins vivos)
│   ├── discover/            # listar placas y resolver -g / GOBLIN_ID / ancestría
│   ├── verbs/               # ParseRef + Mutate, compartidos por los 5 verbos
│   ├── summon/              # lógica de nacimiento inyectable (binarios/rutas)
│   ├── history/             # append de history.jsonl + Compact
│   ├── notify/              # notificaciones JSONL locales (sin red)
│   ├── orb/                 # modelo de la TUI: List/Events/Sampler/Config
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
| `submit` | socket | `verbs.Mutate` → actor → `gg create <id>/step-N/w-I` + `SpawnInto` (`UseCgroupFD`) detrás de `gguard` |
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
- **Hooks:** `on_complete` / `on_fail` al cerrar el paso (con `GOBLIN_LABEL_<KEY>` en el entorno), `on_orphan` (keep/kill/freeze) al morir el instructor, y `on_leftover` (SIGTERM → espera → kill del cgroup del paso).
- **Observabilidad:** `notify_when` publica en `$XDG_RUNTIME_DIR/goblin/notifications.jsonl` (`raven emit <canal> <data>` para uso manual); `then notify via … data=log` usa el `stderr_tail`. Al cerrar un paso se escriben `history.jsonl` y `<id>.history.jsonl`; si un worker falla, `<id>.failures.jsonl` guarda `worker`, `labels`, `exit{code,signal,oom}`, `stderr_tail` (50 KB) y el `status.json` opcional que el job dejó en `GOBLIN_WORKDIR`.
- **Garantía dura:** cada worker corre detrás de `gguard` (`cmd/gguard`), un supervisor sin política que recibe por `ExtraFiles` (fd 3) el extremo de lectura de un pipe cuyo **write end solo sostiene el goblin**. Si el goblin muere de golpe (`SIGKILL`, OOM, crash), el kernel cierra el pipe, el guard ve EOF y mata el grupo del worker con `SIGKILL` — sin depender de que corra código del goblin. No se usa `Pdeathsig`: con el runtime de Go dispara cuando muere el hilo creador, no el proceso. `TestGoblinSIGKILLKillsWorker` valida esto con un `SIGKILL` real. En el cierre limpio, además, el actor hace `gg kill` y borra el manifiesto.

Las reglas viajan como `contract.RuleSpec` (datos) dentro del `Event` de `submit` y el goblin las compila con `contract.CompileAll`; `gutils.Config.Rules` sigue sirviendo como default global. Las variables de entorno `GOBLIN_ORPHAN`, `GOBLIN_NOTIFY`, `GOBLIN_ON_COMPLETE`, `GOBLIN_ON_FAIL` y `GOBLIN_LEFTOVER` configuran las políticas del daemon.

### Nacimiento (`summon`)

`cmd/summon` es el launcher que conecta un script con un contract:

```
summon [--detach] [--name ID] [--memory M] [--cpu C] [--workers N]
       [--orphan keep|kill|freeze] -- CMD [ARGS...]
```

1. `gg discover` → base; `gg create <id>` (+ `gg limit` si hay términos).
2. `os.Pipe()`: la lectura va al goblin por `ExtraFiles` (fd 3), la escritura la retiene `summon`.
3. Lanza `cmd/goblin` con `(id, socket, cgroup, fd, pid_owner, ggPath)` y el env de políticas.
4. Espera el manifiesto, hace `submit` del CMD y espera a que `list` muestre el paso `closed`.
5. Al salir se cierra el pipe: el goblin ve EOF, aplica `on_orphan` y se retira solo.

Con `--detach` se pasa `DEATH_FD=-1` (sin watcher), se imprime el id y se sale sin esperar. Rutas inyectables por `GOBLIN_GG` y `GOBLIN_BIN`.

### DSL `gash` (`.gsh`)

`gash` es un **frontend/prepass sobre `ash`**, no una modificación de su parser: las dos zonas de dispatch (~1048 y ~2774) y la tabla `compoundStarters` (`run→nur` ya venía del fork) quedan intactas, con lo que los 349/349 tests de `ash` no se tocan. El header y los bloques se parsean afuera; los cuerpos de shell se ejecutan con el `ash` real (modo interno `gash --ash FILE`).

```sh
name alineamiento
resources memory=2G cpu=2 workers=2
on_orphan keep

define align
let
    memory=512
    cpu=1
    workers=2
    labels part=1
    abort_when {match 'CUDA out of memory' in stderr}
    fail_when  {gte failed_count 2}
    on_job_exit printf 'ok\n' > /tmp/done
    on_error    printf 'fail\n' > /tmp/fail
    on_leftover term 10
tel

echo inicio
grun align part=2
    echo trabajando
    sleep 0.1
nurg
echo fin
```

- Se aceptan `define` y `def`; `grun <paso>` sin dos puntos; el header se delimita por verbos de header hasta la primera línea que no lo sea.
- **Gramática completa**: hechos `match '<pat>' in stdout|stderr`, `oom`, `oom_count`, `all`/`extreme`, `failed_count`, `time`, `mempeak(level)`, `memmean(level)`, `filesize(ruta)`, `min`/`max` sobre picos de workers; comparadores `gte/gt/lte/lt/eq/neq` con sufijos `K/M/G/s/m/h`; lógicos **prefijos** `and`/`or`/`xor`/`not` con `{}` como única agrupación (sin precedencia implícita). `then notify via <canal> data=<valor>` completo, con `data=log` para el `stderr_tail`. stdout y stderr se capturan por separado para los `match ... in ...`.
- Ejemplo: `abort_when {and {gte failed_count 2} {match 'CUDA out of memory' in stderr}}`
- `contract.RuleSpec` serialize las reglas en el `Event` de `submit`; el goblin las compila con `contract.CompileAll`. Por eso las reglas son datos en la DSL y funciones dentro del daemon.
- Un `.gsh` levanta **un solo goblin** con `summon.StartSession`; cada `grun` es un paso (`step-1`, `step-2`, …) con sus propios recursos, reglas y hooks. Al terminar el script se cierra el pipe y el goblin se retira.

### Comandos de `gg` (implementados en `scripts/gg`)

- `gg discover` — desde `/proc/mounts` y `/proc/self/cgroup`, camina hacia arriba hasta el primer ancestro **escribible y sin procesos internos** (regla *no internal process* de cgroup v2). No mira `user.slice`, no llama a `systemctl`.
- `gg create NAME` — crea el subgrupo y habilita en `cgroup.subtree_control` del padre los controladores disponibles (`cpu`, `memory`, `pids`, `io`, `cpuset`).
- `gg enter NAME PID` — mueve un proceso real al grupo.
- `gg limit NAME memory=2G cpu=1 pids=4` — escribe `memory.max`, `cpu.max` (quota/period) y `pids.max`; acepta sufijos K/M/G.
- `gg freeze|thaw NAME` — `cgroup.freeze` = 1/0 (pausa real, no `SIGSTOP`).
- `gg kill NAME` — `cgroup.kill` del subárbol, con fallback a freeze + kill por PID en kernels < 5.14.
- `gg events NAME [FILE]` — stream de cambios en `cgroup.events` (o `memory.events`) por polling de 1s.
- `gg wait NAME`, `gg stat NAME`, `gg list`, `gg remove NAME`.
- `gg sweep` — borra grupos huérfanos (sin manifiesto y sin procesos) y subdirectorios vacíos. `summon.StartSession` lo llama best-effort antes de crear un goblin, para recuperar cgroups de muertes duras.

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
6. **Nacimiento** (hecho): `pkg/summon` + `cmd/summon` crean cgroup/socket/pipe, lanzan el daemon, hacen submit y esperan el cierre del paso. `TestSummonRunsJobAndRetires` y `TestSummonDetach` cubren ambos modos. Se corrigió una race: `Serve` ahora cuenta las conexiones en vuelo para que el proceso no muera antes de escribir la última respuesta.
7. **`gash`** (hecho): frontend/prepass sobre `ash` (sin tocar su parser): `pkg/gash` parsea header, `define…let…tel` y `grun…nurg`; `RuleSpec` hace las reglas serializables y `summon.StartSession` sostiene un goblin con varios pasos. `TestRunnerEndToEnd` corre un `.gsh` real y verifica cuerpo + hook. Se corrigió `gg create` para habilitar controladores en cada nivel del hierarchy anidado.
8. **Garantía dura** (hecho): auditoría detectó que sin `Pdeathsig` la frase "si el goblin muere, mueren sus workers" solo valía para muertes limpias. Se agregó `cmd/gguard`: supervisor por worker con pipe de liveness, que mata el grupo del hijo al EOF sin código vivo del goblin. `TestGoblinSIGKILLKillsWorker` lo valida con `SIGKILL` real.
9. **Robustez** (hecho): `gutils.Call` tiene `DialTimeout` + deadline de 10s (`TestCallTimeout`), `summon.Launch` cierra la sesión completa en modo normal (sin fugas de cgroup; `--detach` no la cierra), `gg remove` limpia subdirectorios vacíos y `gg sweep` recupera huérfanos al arrancar.
10. **Observabilidad** (hecho): `pkg/notify` + `cmd/raven` publican `notifications.jsonl`; `RuleSpec` lleva `Channel`/`Data` y `data=log` usa el `stderr_tail`; labels en hooks, `history.jsonl` al cerrar y `<id>.failures.jsonl` con `status.json`. `TestObservability` cubre los cuatro frentes con cgroupfs real.
11. **Gramática de reglas** (hecho): `contract.Expr` es un AST serializable con `Eval(Facts)`; `gash` tokeniza y parsea operadores lógicos prefijos con `{}`, agregadores y `filesize`. `Facts` gana `oom_count`, `memmean`, picos por worker y `FileSize` inyectado; el actor captura stdout y stderr por separado. Compatibilidad hacia atrás: las reglas simples siguen llenando `Fact`/`Op`/`Value`.
12. **`orb`** (hecho): TUI estilo `btm` con lo esencial — tabla de goblins y workers con colores por estado, sparklines de CPU/mem, panel de eventos (`history.jsonl` + `notifications.jsonl`) y ayuda. Layout **configurable por archivo, no dinámico** (`$XDG_CONFIG_HOME/orb/config.json`). Cliente puro: consulta cgroupfs/manifiestos y muta por `pkg/verbs`.
13. **Identidad y `step_tree`** (hecho): `ReadStat` expone `starttime` y el manifiesto guarda `(pid,starttime)` del goblin y del instructor; `discover.Resolve` compara por identidad, no solo PID. `TreeSampler` muestrea el árbol por cambio de `populated` (poll 250ms) con resguardo de 5 min, identifica procesos por `(pid,starttime)`, trunca `argv0` a 64, limita a 500 y guarda `step_tree` en `<id>.history.jsonl` al cerrar el paso.
14. **`Ancestors`/`Subtree`** (hecho): `gprocutil.Identity{PID,Starttime}` + `IdentityOf`; `Ancestors` sube al padre sin incluir el propio PID ni init, con guarda de ciclos; `Subtree` hace BFS incluyendo la raíz con un solo pase por `/proc`. `discover.Resolve` dejó de hacer su propio loop y usa `Ancestors`.
15. **`Event.Error` tipado** (hecho): `Error` queda solo para `Type:"error"`; `list` devuelve `Steps []StepInfo{ID,State,Outcome,Workers}`, `output` usa `Line`, y `death` usa `Message`. `summon.WaitClosed` y los tests consultan `Steps` en vez de parsear un string con `":closed:"`. Campos con `omitempty`: protocolo compatible.
16. **`on_unborn` / `on_headless`** (hecho): el parser guarda y valida las tres políticas del header. `gguard` aplica `on_headless` al cerrarse el pipe de liveness (`kill`→SIGKILL, `freeze`→SIGSTOP, `keep`→sin señal) leyendo `GOBLIN_HEADLESS`. `summon.applyUnborn` reacciona al fallo de nacimiento (`kill` limpia, `freeze` congela, `keep` deja) y siempre escribe `channel:"unborn"` en `notifications.jsonl`. Flags `--unborn`/`--headless` en `summon`.
17. **`cpu` fraccionario** (hecho): `gg limit cpu=1.5` acepta decimales (validación con `awk`, quota = `cpu×100000` redondeado) y escribe `150000 100000` en `cpu.max`. La capa Go dejó de truncar con `int()` y usa `fmt.Sprintf("cpu=%g", ...)` en `actor` y `summon`. `cpu=0` sigue significando "sin límite".
18. **Limpieza de código muerto** (hecho): se borró `runRunScript` y su entrada `run→nur` del parser de `ash` (nunca se llamaba; `gash` hace el prepass afuera), `gutils.SocketPath()`, `MaxWPerCPU` y `swap` (declarados pero sin efecto), el campo `Command` legacy de `RuleSpec`/`Rule` (vector de `sh -c` sin uso) y los build tags WASM de 11 archivos. Los 349/349 de `ash` siguen pasando.
19. **Fachada y superficies** (pendiente): README/Makefile definitivos, archivar handoffs; `gobblr` (R) diferido, sin `warlock` (el goblin es el daemon efímero).

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
go test ./...           # unit tests (47 paquetes verdes)
go vet ./...
make build              # atajo del Makefile
make test-gg            # prueba end-to-end de ggroups contra cgroupfs real
go test ./pkg/contract -v                          # estados, reglas, hooks (lógica pura)
go test ./pkg/gutils -v                            # daemon + abort_when real sobre cgroupfs
go test ./pkg/summon -v                            # nacimiento: submit + espera + detach
go test ./pkg/gash -v                              # DSL: parser + .gsh end-to-end
go test ./pkg/notify ./pkg/gutils -run 'TestEmit|TestObservability' -v   # notificaciones, labels, historia, fallos
./scripts/gg discover                              # ver el ancestro cgroup2 que usa el sistema
# binarios: cmd/gash (DSL), cmd/summon (launcher), cmd/goblin (daemon), cmd/gguard (liveness), cmd/raven (notify), cmd/orb (TUI), cmd/grimoire (applets)
# orb                                       # TUI: j/k, p/r/x, ?, q
# config: $XDG_CONFIG_HOME/orb/config.json  # {"widgets":["top","goblins","workers","events"],"heights":{...}}
# raven emit email 'job terminado'   # escribe en $XDG_RUNTIME_DIR/goblin/notifications.jsonl
# ejemplo: summon -- sh -c 'echo hola'
#          summon --detach --name demo -- sleep 300
#          gash pipeline.gsh
```

---

## Licencia

MIT (heredada del fork de `go-busybox`).
