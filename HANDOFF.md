# goblin — handoff

## Objetivo del proyecto

`goblin` es un motor de ejecución de *jobs* computacionales pesados (bioinformática,
entrenamiento de ML, cómputo científico, geoespacial) que aplica al espacio de
procesos Unix una semántica inspirada en los **contracts** de illumos: cada
job tiene un ciclo de vida observable (estados, eventos, términos de recurso),
en vez de ser un proceso anónimo que se lanza y se pierde.

La pieza central es el **goblin**: una estructura que se comporta como un daemon
(gestiona el ciclo de vida de un conjunto de procesos) pero que **no es
persistente** — nace con el script que lo invoca y muere con él. La garantía
dura del sistema es: **si el goblin muere, todo lo que gestiona muere con
él** — sin excepciones, sin huérfanos a nivel de sistema operativo.

Encima de ese motor se apoyan tres superficies de uso:

1. **Una DSL en `sh` POSIX puro** (`.gsh`), pensada para que pipelines de
   bioinformática/geoespacial/ML se escriban como scripts de shell normales
   (`ash`/`dash`/`bash`, sin bashismos), con un puñado de verbos nuevos
   (`def`, `resources`, `run...nur`, `on_complete`, `on_fail`) en vez de un
   lenguaje de configuración aparte.
2. **Un TUI de gestión** estilo `bottom` — descubre todos los goblins vivos
   del usuario, permite pausar/reanudar/matar, y muestra un historial corto
   de los que ya terminaron.
3. **`gobblr`**, un paquete R (fork de `mirai`) que embebe el mismo engine
   vía cgo (`-buildmode=c-shared`), para que R pueda paralelizar jobs con
   las mismas garantías de contrato — pausa/resume real y recuperación de
   subárboles, cosa que ni `future` ni `mirai` ofrecen hoy.

---

## Resumen: qué tomamos, qué sacamos, qué buscamos hacer

### Qué tomamos

Se hizo **hard fork** de [`rcarmo/go-busybox`](https://github.com/rcarmo/go-busybox)
(licencia MIT), una reimplementación de BusyBox en Go con ~57 applets,
validada contra el test suite real de BusyBox (349/349 en `ash`).

La razón de forkear en vez de escribir desde cero: el fork ya trae **un
parser de `ash` real y probado** (~5.700 líneas), y varios applets de
gestión de procesos (`kill`, `ps`, `pgrep`, etc.) con la lógica de
`/proc`-parsing ya resuelta y testeada. Reescribir eso no aportaba nada al
proyecto — el valor de goblin no está en "saber matar un proceso", está en
la capa de contrato encima.

### Qué eliminamos

Del árbol original (~57 applets) se podó a mano hasta dejar **37**:

**Eliminados** (fuera de dominio):
- Networking: `dig`, `nc`, `ss`, `wget`
- Sesión/sistema interactivo: `logname`, `users`, `w`, `who`, `whoami`, `watch`, `ionice`
- `top` (el TUI de goblin lo reemplaza directamente)
- El dispatcher multi-call genérico `cmd/busybox` y los `cmd/<applet>`
  individuales de todo lo de arriba

**Se mantienen** (37, dos roles):
- *Motor*: `ash` (a extender con la gramática de `.gsh`), `kill`/`killall`/
  `pgrep`/`pidof`/`pkill` (señales), `ps` (métricas vía `/proc`), `nice`/
  `renice`/`taskset` (afinidad/prioridad), `setsid`/`nohup`/`startstopdaemon`
  (process-group management), `timeout`, `free`/`nproc`/`uptime` (info de nodo)
- *Stdlib de scripting* para que un `.gsh` no dependa del sistema host:
  `awk`, `sed`, `grep`, `cut`, `sort`, `uniq`, `wc`, `tr`, `head`, `tail`,
  `diff`, `tar`, `gzip`, `gunzip`, `cp`, `mv`, `rm`, `rmdir`, `mkdir`, `find`,
  `ls`, `pwd`, `cat`, `echo`, `printf`, `time`, `sleep`, `xargs`

**Se agregan** (nuevos, no vienen del fork):
- `procutil.Children`/`Descendants` — árbol de procesos vía `PPID`, reusando
  `ReadStat` (ya trae `PPID`, no agrega lecturas nuevas de `/proc`)
- Applet **`pstree`** — expone ese árbol como comando, mismo patrón
  `Run(stdio,args)` que todo lo demás
- Applets **`goblinsubmit`/`goblinpause`/`goblinresume`/`goblinkill`/
  `goblinlist`** — un applet por acción (decisión explícita: ni un `goblin`
  con subcomandos, ni un `gdaemon` separado), clientes NDJSON delgados sobre
  `pkg/goblin.Call`/`SimpleVerb`
- `pkg/goblin.Spawn`/`Serve` — el fork/exec real (con `setpgid`) y el
  servidor NDJSON del lado goblin; cierran el hueco de "`AttachWorker`
  registraba un pid pero nadie lo creaba"

El Makefile del fork se reescribió completo: se sacó todo lo de `tinygo`/WASM
(no hay caso de uso de sandboxing acá), y se agregaron targets propios
(`verify-applets`, `prune-check`, `sizes`, `build-cshared`).

### Qué buscamos hacer con ese componente acortado

El árbol podado es la base sobre la que se construye, **sin duplicar lo que
ya existe ahí**: cuando `pkg/goblin` necesita matar un process group, llama
`syscall.Kill(-pgid, sig)` igual que ya hace `kill.go`; cuando necesita RSS
por proceso, llama a la misma lógica de `/proc/PID/stat` que ya usaba `ps.go`
(promovida a `procutil.ReadStat` para poder compartirse entre paquetes, ya
que antes era una función no exportada dentro de `ps`). La regla de diseño
explícita es: **todo lo que sea "inspeccionar o señalizar procesos" es
mecanismo heredado del fork; todo lo que sea "qué significa cada señal en
términos de contrato" es política nueva, vive en `pkg/goblin`.**

---

## Qué falta (lógica pendiente)

| Pieza | Estado | Qué falta |
|---|---|---|
| **`pkg/goblin`** (core: `Terms`, `Event`, estados, cascada, `CheckLimits`, `Snapshot`) | primera versión escrita, sin compilar/testear todavía contra el repo real | confirmar que compila contra los paths de import reales del fork |
| **`pkg/goblin`: `Spawn`/`Serve`** (fork/exec real + servidor NDJSON sobre unix socket) | diseñado, primera versión escrita | cerraba el hueco más grande del handoff anterior (`AttachWorker` no lanzaba nada); falta manejo de `on_complete`/`on_fail` al recibir el evento terminal, y logging real en vez de heredar stdout/stderr del goblin |
| **`procutil`: `Children`/`Descendants`** (árbol de procesos vía `PPID`) | diseñado, primera versión escrita | necesario para la recuperación de subárbol (worker-padre muere, sus hijos no deberían morir) — sin esto, goblin solo conocía el pid líder de cada worker, no su descendencia real |
| **Applet `pstree`** | diseñado, primera versión escrita | expone el árbol como comando standalone, útil para debug manual además de uso interno de goblin |
| **Applets de verbos** (`goblinsubmit`, `goblinpause`, `goblinresume`, `goblinkill`, `goblinlist`) | decidido: **un applet por acción**, no un solo `goblin` con subcomandos ni un `gdaemon` separado — mismo patrón `Run(stdio,args) int` que el resto del árbol podado | clientes NDJSON delgados sobre `pkg/goblin.Call`/`SimpleVerb`; falta terminar de escribirlos y que `cmd/goblin` los dispatchee |
| **Extensión del parser `ash`** | no empezada | agregar los verbos `def`, `resources`, `memory`/`cpu`/`gpu`, `workers`, `max_w_per_cpu` como funciones/estado del intérprete; y sobre todo, el token `nur` como cierre real de bloque `run...nur` (en vez del workaround actual con heredoc `<<'NUR'`, que ya funciona pero es una concesión a POSIX puro, no la sintaxis final que se quiere) |
| **Detección de muerte del script (pipe-EOF)** | mecanismo validado por separado (funciona incluso con `SIGKILL` al padre) | falta integrarlo: quién crea el pipe, quién le pasa el fd al goblin, en qué punto exacto del flujo `ash` extendido → `pkg/goblin` |
| **`pkg/history`** | diseñado (escritura atómica de línea final a `history.jsonl`), no implementado | — |
| **`pkg/discover`** | diseñado (enumerar sockets en `$XDG_RUNTIME_DIR/goblin/`), no implementado | hoy `goblin.SocketPath()` usa un fallback simple basado en `PPID`; `pkg/discover` es lo que reemplaza eso por descubrimiento real multi-goblin para el TUI |
| **`cmd/goblin`** (entrypoint multi-call real) | no existe — el `cmd/busybox` original se borró junto con la poda | dispatcher que reemplaza al de busybox, apuntando a los 37 applets podados + los 5 nuevos de verbos + `pstree`, y el modo `.gsh` |
| **`cmd/goblin-tui`** | diseño de layout pendiente (paneles: lista de goblins, recursos agregados, historial reciente) | todo — ni siquiera el esqueleto |
| **`internal/cshim`** (API C para embeber en R) | firma propuesta (`goblin_spawn/pause/resume/kill/poll`), no implementada | falta resolver el manejo de señales del runtime Go embebido (`GODEBUG=asyncpreemptoff=1`, evitar `os/signal.Notify` dentro de la librería) |
| **`bindings/gobblr`** (paquete R, fork de `mirai`) | decidido conceptualmente (qué partes de `mirai` se reusan vs. reemplazan), no empezado | requiere que `internal/cshim` exista primero |
| **`jc.sh` / stdlib de conveniencia del `.gsh`** (helper `over` para condiciones de tamaño de archivo, `notify_email`, etc.) | prototipados y probados en `dash` puro como funciones sh | portar a la gramática nativa de `ash` extendido una vez esa pieza exista, en vez de vivir como funciones `sh` externas |

## Cómo se quiere abordar lo que falta (orden sugerido)

1. **Cerrar `pkg/goblin`** contra el repo real — compilar, confirmar que
   `Spawn`/`Serve` (recién agregados) funcionan de punta a punta con un job
   de juguete.
2. **`cmd/goblin`** mínimo — que compile y dispatchee a los 37 applets
   podados + los nuevos (`pstree`, `goblinsubmit`/`pause`/`resume`/`kill`/
   `list`), para tener un binario real sobre el cual iterar.
3. **`pkg/discover` + `pkg/history`** — desbloquea al mismo tiempo el TUI
   y la integración con `gobblr` más adelante (el protocolo NDJSON en sí
   ya tiene primera versión escrita como parte de `pkg/goblin`).
4. **Extensión de `ash`** — el trabajo más largo, se puede hacer en paralelo
   a lo anterior porque no depende de ello (la gramática y el core de
   contrato son ortogonales).
5. **TUI** — una vez el protocolo y el discovery estén firmes.
6. **`cshim` + `gobblr`** — al final, porque depende de que `pkg/goblin`
   ya esté estable (cambiarlo después de embeberlo en R es más caro).
