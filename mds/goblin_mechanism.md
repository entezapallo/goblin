# goblin_mechanism — mecánica interna del goblin (v1)

Reemplaza a `ancestry-spec.md`. Complementa a `ggroups-spec.md` (que cubre la mecánica del
kernel/cgroups). Este documento cubre lo que pasa **dentro** del proceso goblin: cómo
modela su propio subárbol, quién puede instruirlo, su máquina de estados, cómo evalúa
reglas sobre stdout/stderr, los applets CLI que hablan con él, y qué se reusa del árbol de
`busybox` podado.

## 0. Cómo leer este documento

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Agregado para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |
| **[SIN VERIFICAR]** | Supuesto sobre el entorno o el código existente sin probar. |

---

## 1. El goblin es un subárbol

**[DECIDIDO]** Un goblin es un subárbol: la representación de un grafo de ancestría. El
ggroup (cgroup) hace cumplir límites y muertes; el grafo describe la estructura.

### 1.1 Nodos

Cada proceso es un nodo, identificado por **`(pid, starttime)`** (nunca solo el PID, por si
se recicla). Atributos: `ppid` real, `argv[0]`, hoja del ggroup, rol, estado, `first_seen`,
`last_seen`.

| Rol | Qué es |
|---|---|
| `instructor` | quien dirige al goblin (ver §2) |
| `goblin` | el proceso goblin |
| `idproc` | el proceso de id |
| `worker` | proceso de cómputo lanzado por el goblin |
| `descendant` | cualquier descendiente de un worker |

### 1.2 Aristas y raíz

- **`ppid`** (real): el padre en el kernel.
- **`lparent`** (lógica): el padre en el grafo. Coincide con `ppid` salvo en el goblin, cuyo
  `lparent` es el instructor activo (su `ppid` real es `warlock`).

**[DECIDIDO]** La raíz lógica es el **instructor**; el goblin y sus workers cuelgan de él.

```
gsh / R / terminal   (instructor, raíz lógica)   ← ppid real = warlock
└── goblin
    ├── idproc
    └── step-0001
        ├── w-1  (snakemake)
        │   └── bwa
        └── w-2
```

**[ASUMIDO]** Sin instructor (`summon --detach`, o instructor ya muerto): la raíz es el
proceso goblin.

**Limitación conocida:** la raíz lógica no es ancestro real por `ppid`. Un recorrido por
`ppid` real desde un worker llega al goblin y de ahí a `warlock`, no al instructor. Por eso
el instructor se busca en el manifiesto, no subiendo por el árbol real.

### 1.3 Membresía y anomalías

`cgroup.procs` de `<id>/` (recursivo) es la fuente **autoritativa** de pertenencia; el árbol
por `lparent` da la estructura. **[ASUMIDO]** Un proceso en el ggroup cuya cadena de
ancestros no llega al goblin se marca `detached_member` — es lo que pasa tras la muerte del
goblin (re-parentados a `init`): el árbol se rompe en un bosque, y esa es la representación
de "sin cabeza" (`ggroups-spec.md` §8.8).

### 1.4 Cómo se calcula

**[ASUMIDO]** Bajo demanda, sin estado persistente propio: leer `cgroup.procs` de todas las
hojas de `<id>/`, leer `/proc/PID/stat` (ppid, starttime) y `/proc/PID/cmdline` por cada
PID, agregar el nodo del instructor desde el manifiesto, armar el árbol por `lparent`.

**Reuso.** Sobre `gprocutil`: `ReadStat`, `Children`, `Descendants` ya existen
(**[SIN VERIFICAR]**: que `ReadStat` exponga `starttime` y `ppid`). Faltan `Ancestors(pid)`
y `Subtree(rootPID)`, sobre la misma base. `Descendants` sigue siendo estructura base; lo
que desaparece es `killTree` (§7), reemplazado por `cgroup.kill`.

**Tamaño.** El verbo `state` incluye el grafo solo si se pide (`graph: true`).
**[ASUMIDO]**

---

## 2. El instructor genérico

**[DECIDIDO]** No hay tipos de instructor con reglas especiales cada uno (nada de
`kind: gsh` vs `kind: gobblr` con comportamiento distinto hardcodeado). El goblin expone un
único protocolo de instrucción; cualquiera que le hable por ahí es un instructor: una
terminal (`summon`), una sesión de R (`gobblr`, que "finge" ser un system shell script vía
`system()`/`system2()`, igual que cualquier otro proceso Unix), o `gsh`.

### 2.1 Qué manda cada instructor al conectarse

**[DECIDIDO]** Al conectarse, el instructor manda su propia configuración; el goblin no
infiere nada de un tipo fijo:

```json
{
  "attach": true,
  "watch_pid": 4821,
  "watch_starttime": 918273,
  "kind": "gsh"
}
```

- **`attach`:** si el instructor se mueve dentro del ggroup (a `owner/`, ver §2.2). `gsh`:
  sí (decidido). R: no — si la sesión de R se moviera adentro, `banish --all` la mataría y
  los límites de memoria/cpu la alcanzarían.
- **`watch_pid` / `watch_starttime`:** identidad a vigilar por `pidfd` para disparar
  `on_orphan` si muere de golpe.
- **`kind`:** metadato sin efecto en el comportamiento — solo para logs y para que `orb`
  muestre "lanzado desde R" vs "desde un script". **[DECIDIDO]**

### 2.2 `owner/` reemplaza a `shell/`

**[DECIDIDO]** Renombre respecto a `ggroups-spec.md`: la hoja `shell/` pasa a llamarse
`owner/`, y solo existe si `attach: true`. `pidgsh` del id pasa a `pidowner`; en el
manifiesto, los campos `gsh_*` pasan a `owner_*`.

```
<base>/goblin/<id>/
├── self/
├── owner/      (solo si attach=true; hoy: gsh)
└── step-0001/
    └── w-i/
```

### 2.3 Resolución por ancestría

Ver §6 (direccionamiento de applets) para el algoritmo completo. Aplica igual sin importar
el `kind` del instructor.

### 2.4 Pendiente explícito

**[ABIERTO]** El detalle completo de `gobblr`/R (cómo `attach: false` interactúa con
`on_orphan`, cómo R recibe eventos sin hooks de shell) queda fuera de alcance — es la
sesión de `gobblr`, marcada como "sin apuro" desde el handoff original. Lo que este
documento fija es solo el contrato genérico que R deberá cumplir cuando se diseñe.

---

## 3. `summon`

**[DECIDIDO]** `summon` no es un tipo de instructor aparte — es la CLI actuando como
instructor. Crea un goblin de un solo paso.

```
summon [--detach] [--name N] [--memory M] [--cpu C] [--workers N] [--pids N]
       [--swap S] [--labels k=v,...] [--on-leftover term:S] -- CMD [ARGS...]
```

- Sin `--detach`: la terminal es el instructor (`attach: true`, `watch_pid` = la terminal),
  bloquea y devuelve el código de salida del job. Si cierras la terminal, aplica
  `on_orphan` como cualquier goblin.
- Con `--detach`: el goblin nace sin instructor (raíz lógica = el propio goblin), nadie lo
  vigila, vive hasta que termina su único paso, y `summon` devuelve enseguida el id.

**[DECIDIDO]** No existe una forma de "sumarse" a un goblin existente (agregar pasos desde
afuera) — rompería la idea de que el instructor lleva el flujo del paso activo.

---

## 4. Máquina de estados

Tres niveles anidados: goblin → paso → worker. Cada nivel dispara transiciones en el de
arriba. **[ASUMIDO]** salvo donde se marque lo contrario.

### 4.1 Goblin

```
unborn ──(warlock crea proceso, self/ listo)──▶ alive
alive ──(run llega)──▶ running
running ──(paso cierra)──▶ alive
alive/running ──(owner_done, on_headless, banish --all)──▶ dying
dying ──(ggroup vacío)──▶ dead
```

| Estado | Qué significa | Quién puede mirarlo |
|---|---|---|
| `unborn` | `.birth` existe, `.json` no | `warlock` (timeout 5s, `ggroups-spec.md` §8.1) |
| `alive` | goblin vivo, sin paso activo | `scry` |
| `running` | un paso activo | `scry`, `orb` |
| `dying` | limpiando, esperando cgroup vacío | no acepta `run` nuevo |
| `dead` | terminado, `warlock` ya borró todo | solo queda en `history` |

### 4.2 Paso (`step-N`)

```
pending ──(admisión OK)──▶ starting ──(cgroup + workers listos)──▶ active
active ──(todos los workers terminaron)──▶ draining
draining ──(on_leftover resuelto)──▶ closing
closing ──(outputs publicados, cgroup borrado)──▶ closed{success|failed|aborted}
active ──(abort_when se cumple)──▶ aborting ──▶ draining
pending ──(admisión falla)──▶ closed{rejected}   (nunca crea cgroup)
```

`aborting` es sub-estado de `active`: ya se mandó `cgroup.kill` a los workers vivos, falta
reapear y clasificar a los que ya habían salido.

### 4.3 Worker (`w-i`)

```
spawning ──▶ running ──▶ exited{code}
                     ├─▶ killed{signal}      (banish, abort_when, on_exit=kill)
                     └─▶ oom_killed
```

Los tres finales se reducen a **éxito** (código 0) o **fallo** (todo lo demás), regla de
`ggroups-spec.md` §7.3/§8.3.

### 4.4 Cuándo se evalúa cada regla

| Evento | Dispara |
|---|---|
| `worker_exit` (cualquiera) | reevaluar `abort_when` y `notify_when` sobre el paso |
| línea nueva de stdout/stderr | reevaluar `abort_when`/`notify_when` que miran texto (§5) |
| `tick` periódico (1s, mientras `active`) | reevaluar reglas con el hecho `t` |
| todos los workers terminaron | evaluar `fail_when` una sola vez → `closed{success\|failed}` |

`abort_when`/`notify_when` se evalúan en caliente, sobre eventos. `fail_when` se evalúa una
vez al cerrar, porque necesita el resultado final de todos los workers.

---

## 5. Motor de reglas

**[DECIDIDO]** Tres verbos: `fail_when` (marca el paso fallido al cerrar), `abort_when`
(fail-fast: mata al resto ya), `notify_when` (no mata ni falla, solo notifica; nuevo
respecto a las rondas anteriores).

### 5.1 Texto: Go nativo, no applets

**[DECIDIDO]** Coincidencia de patrones sobre stdout/stderr (p. ej. "abortar si 3 workers
imprimen `CUDA out of memory`") se evalúa con `regexp`/`strings` de la librería estándar de
Go, en la misma goroutine que ya lee la línea para escribirla a archivo y llenar
`stderr_tail` (§8). Sin subproceso, sin protocolo de ida y vuelta.

`awk`/`grep`/`sed`/`pgrep` siguen disponibles en el PATH para que **el propio job** del
usuario los use dentro de su script — eso ya funciona hoy, sin diseño adicional, porque son
applets normales de `grimoire`. No forman parte del motor de reglas del goblin.

### 5.2 `notify_when`

**[DECIDIDO]** Enfriamiento configurable, default **1 minuto**: no repite el mismo aviso
antes de que pase ese tiempo desde el último disparo de la misma regla. Puede dispararse
muchas veces durante el mismo paso (a diferencia de `abort_when`, que actúa una sola vez y
termina el paso). Llama a `raven emit` (ver `ggroups-spec.md` §10, reformulado por
`raven` como binario propio en la sesión de protocolos).

### 5.3 Sintaxis y "hechos"

**[ABIERTO]** — territorio compartido con la sesión de `gash`. Hechos conocidos hasta
ahora: `oom`, `mem_over`, `t` (tiempo desde inicio del paso), % de workers fallidos, y
coincidencias de texto en stdout/stderr (§5.1). El significado exacto de `min` en la nota
original nunca se aclaró; queda para esa sesión.

---

## 6. Direccionamiento de los applets de verbo

Los applets (`summon`, `banish`, `scry`, `slumber`, `awaken`) resuelven dos cosas: **a qué
goblin** hablan y **a qué parte** aplican.

### 6.1 A qué goblin

**[DECIDIDO]** Orden de resolución:

1. `-g <ref>` explícito.
2. La variable `GOBLIN_ID`.
3. Ancestría (§6.2).
4. Error, listando candidatos.

`ref`: id completo, prefijo único, o `name`. **[ASUMIDO]** Ambiguo → error con candidatos;
existe `--latest`.

**Quién exporta `GOBLIN_ID` [ASUMIDO]:** el instructor, al recibir el id tras el nacimiento;
el goblin, en el entorno de cada worker (`ggroups-spec.md` §11).

**Reglas de conflicto [ASUMIDO]:**
- `GOBLIN_ID` apunta a un goblin que no existe: se ignora con aviso, sigue la ancestría.
- `GOBLIN_ID` y ancestría apuntan a goblins distintos: para `banish`/`slumber`/`awaken`,
  error que pide `-g`; para `scry`, gana la variable con aviso.
- Los verbos destructivos **nunca adivinan** ("hay uno solo, lo uso"): solo valen 1 a 3.

### 6.2 Resolución por ancestría

```
cur = mi propio proceso
mientras cur != init:
    si (pid, starttime) de cur es un proceso goblin en algún manifiesto: devolver ese goblin
    si (pid, starttime) de cur es el instructor en algún manifiesto:    devolver ese goblin
    cur = ppid real de cur
```

Se comparan manifiestos de `$XDG_RUNTIME_DIR/goblin/*.json`. Scripts anidados: gana el
ancestro más cercano. **[SIN VERIFICAR]** que la cadena de `ppid` sobreviva a `sudo`,
`env -i` y envoltorios similares.

| Desde dónde se llama | Ancestro que resuelve |
|---|---|
| un worker o su descendiente | el proceso goblin |
| un comando corrido por el instructor | el instructor, y el manifiesto da el goblin |
| otra terminal, sin relación | ninguno: solo `-g` |
| lanzado con `env -i` | variable perdida, ancestría funciona |
| lanzado antes del primer `run` | ninguno: el goblin no nació |

### 6.3 Cómo listan

`scry` sin target: **[ASUMIDO]** pregunta primero a `warlock`; si no responde, escanea las
placas (`pkg/discover` sobre `<id>.json` + intento de conexión con timeout a cada socket).
El goblin sobrevive a `warlock`, así que los applets no dependen de él.
`scry -g X` pide `state` al goblin. Salida en tabla; `--json` para scripts y R.

### 6.4 Alcance (separado del target)

Corrige `ggroups-spec.md` §8.5, que mezclaba target con alcance:

- `banish [--step N | --worker I | --all]`: sin argumento, el paso activo; `--all` mata el
  ggroup entero (incluido `owner/` si `attach=true`).
- `slumber`/`awaken`: el paso activo, o `--step N`. Nunca el goblin entero.

---

## 7. Applets CLI

Comparten `-g <ref>` y `--json`. Devuelven código de salida distinto de cero si el applet
mismo falla (no confundir con el código de salida del job).

```
summon [--detach] [--name N] [--memory M] [--cpu C] [--workers N] [--pids N]
       [--swap S] [--labels k=v,...] [--on-leftover term:S] -- CMD [ARGS...]

banish -g <ref> [--step N | --worker I | --all]

slumber -g <ref> [--step N]
awaken  -g <ref> [--step N]

scry                          # lista todos los goblins
scry -g <ref>                 # state: pasos, workers, recursos
scry -g <ref> --graph         # incluye el árbol de ancestría
scry -g <ref> --follow        # se suscribe, sigue eventos en vivo
scry -g <ref> --step N --logs # sigue stdout/stderr de un paso
```

**[DECIDIDO]** No hay applet `abort` aparte: "abortar" es `banish`. No hay applet para
consultar reglas activas más allá de lo que `scry -g <ref>` ya muestra en su `state`.

---

## 8. Struct de depuración (fallo de un worker)

**[DECIDIDO]** Un solo struct, cruce de etiquetas explícitas + cola de stderr, con
`status.json` opcional que complementa (no reemplaza) lo automático:

```json
{
  "worker": "w-1",
  "labels": {"part": "1", "rows": "200-400"},
  "exit": {"code": 1, "signal": null, "oom": false},
  "stderr_tail": ["Warning: 340 NA values found", "Error: model did not converge"],
  "status": {"ok": false, "reason": "muchos NA"}
}
```

- **`labels`:** declaradas explícitas, parámetro de `run` (p. ej.
  `run alinear labels part=1,rows=200-400 ... nur`). No se intentan adivinar de los
  argumentos del comando. **[DECIDIDO]**
- **`stderr_tail`:** los últimos **50 KB** (no líneas) del stderr del worker, guardados con
  el evento de fallo. **[DECIDIDO]**
- **`status`:** si el job escribe `status.json` en su `GOBLIN_WORKDIR`
  (`ggroups-spec.md` §11) antes de salir, el goblin lo lee y lo funde con lo automático.
  Opcional, complementario. **[DECIDIDO]**

---

## 9. Historial del árbol (muestreo)

**[DECIDIDO]** El árbol de cada paso se muestrea **por evento**, con un resguardo por
tiempo si está idle. Motivo dado: permitir depurar cosas como "partición 1, filas 200-400,
no convergió porque había muchos NA" viendo qué corrió, cruzado con el struct de §8.

### 9.1 Disparo

**[DECIDIDO]** El goblin se suscribe (`inotify`/`poll`) al campo `populated` de
`step-N/cgroup.events`. Cada cambio (`0→1`, `1→0`, o cualquier variación que el kernel
reporte) dispara un muestreo del árbol y reinicia un contador de tiempo idle a cero. Si
pasan **5 minutos** (configurable, **[DECIDIDO]**) sin que ese archivo cambie, se dispara
una foto de respaldo igual, y el contador se reinicia.

No hay un "intervalo normal" separado del evento: el evento por `populated` **es** el
intervalo normal; el reloj de 5 min es solo el resguardo para cuando no hay eventos (p. ej.
un único proceso largo que nunca crea ni pierde hijos).

### 9.2 Qué se registra

**[ASUMIDO]** Por proceso nuevo visto: `(pid, starttime)`, `ppid`, `argv[0]` truncado a 64
caracteres, índice del worker (derivado de la hoja `w-i/`), `first_seen`, `last_seen`. Tope
de 500 procesos por paso; el resto se cuenta pero no se detalla. Se guarda como registro
`step_tree` en `<id>.history.jsonl` al cerrar el paso.

**Limitación honesta:** el muestreo no ve procesos de vida menor al intervalo efectivo entre
eventos; un registro completo exigiría eBPF o `auditd`, con privilegios que este diseño no
asume.

---

## 10. Reuso de applets heredados (ítem 4, resuelto)

Revisado contra el código real de `kill.go`, `killall.go`, `startstopdaemon.go`, `pgrep.go`.
El diagnóstico del handoff original ("señales y process-group management ya resueltos, solo
falta exportar") era parcialmente incorrecto: solo una parte de eso existe de verdad en el
fork.

### 10.1 Qué hay

| Applet | Qué expone hoy | Rol en `goblin` |
|---|---|---|
| `kill.go` | `procutil.ParseSignal`, `procutil.SignalNames()` — ya exportadas, reuso duro | ninguno directo: `ggroups` reemplaza señal-por-PID por `cgroup.kill`/`freeze` |
| `pgrep.go` | `procutil.MatchProcs`, `procutil.SortByPID` — exportadas, reuso duro | **`warlock`**, en reconciliación (§10.3) |
| `killall.go` | nada propio; delega todo a `pgrep.Run` | ninguno; queda como applet CLI independiente |
| `startstopdaemon.go` | `exec.Command(...).Start()` desnudo; `--pidfile` se acepta pero **nunca escribe nada** (bug) | base de `gutils.Spawn` (§10.2), tras agregarle `SpawnInner` |

### 10.2 `gutils.Spawn` → `startstopdaemon.SpawnInner`

**[DECIDIDO]** `gutils.Spawn` (el actor de `goblin`) llama **literalmente** a una función
nueva en el paquete `startstopdaemon`, no reimplementa `exec.Command` por su cuenta.

Dos firmas en el mismo paquete, separando lo que puede pedirse por CLI de lo que no:

```go
// startstopdaemon/spawn.go

// SpawnInner: con poderes. Solo gutils.Spawn la llama. No hay forma de
// pedir CgroupFD o Pdeathsig desde args []string — el candado es la firma
// tipada en sí, no una verificación en tiempo de ejecución.
func SpawnInner(cfg SpawnConfig) (*os.Process, error) {
    cmd := exec.Command(cfg.Binary, cfg.Args...)
    cmd.SysProcAttr = &syscall.SysProcAttr{
        UseCgroupFD: true,
        CgroupFD:    cfg.CgroupFD,
        Pdeathsig:   syscall.SIGKILL,
    }
    if err := cmd.Start(); err != nil {
        return nil, err
    }
    return cmd.Process, nil
}

// Run: la CLI de siempre, sin acceso a CgroupFD/Pdeathsig.
func Run(stdio *core.Stdio, args []string) int {
    cfg, err := parseArgs(args)
    // cfg.CgroupFD siempre 0; no hay flag que lo setee
    proc, err := SpawnInner(cfg)
    // imprime PID, códigos de salida, igual que hoy
}
```

**El bug de `--pidfile` queda descartado como mecanismo, no como código a arreglar.** El
manifiesto (`<id>.json`, con PID + `starttime` + ruta del ggroup) cumple el mismo rol que un
pidfile, de forma más completa. Arreglar el bug (si se hace) es higiene del árbol de
`busybox`, sin relación con `goblin_mechanism`.

### 10.3 `pgrep` en la reconciliación de `warlock`

**[DECIDIDO]** Amplía `ggroups-spec.md` §9 (tabla de reconciliación): `warlock` también
busca, vía `procutil.MatchProcs`, procesos con `argv[0]` que matchea el patrón
`goblin:*` (recordar `ggroups-spec.md` §8.1: `argv[0] = "goblin:<id>"`) que **no** están en
ningún ggroup conocido. Cubre el caso de un goblin cuyo cgroup se corrompió o quedó fuera
del árbol reconocido, más allá de lo que ya detectan los manifiestos y `cgroup.procs`.

**[ASUMIDO]** Fila nueva en la tabla de reconciliación:

| Proceso goblin | ggroup | Estado | Acción |
|---|---|---|---|
| encontrado por `pgrep` sobre `goblin:*`, sin ggroup reconocible | — | corrupto/huérfano de mecanismo | notificar; no matar automáticamente |

### 10.4 Resultado neto sobre `gutils`

- `Spawn`: deja de reimplementar `exec.Command` a mano; pasa a ser un wrapper delgado sobre
  `startstopdaemon.SpawnInner`.
- `signalWorker`, `killTree`: **desaparecen**, reemplazados por `cgroup.kill`/
  `cgroup.freeze` de `ggroups` (`ggroups-spec.md` §7-8).
- No hay applet nuevo que envolver para el `SysProcAttr.Setpgid` de antes — ya no aplica:
  con `UseCgroupFD`, la agrupación la da el cgroup, no `setpgid`.

---

## 11. Efecto sobre otros documentos

- **`ggroups-spec.md`:** §3 y §8 deben renombrarse (`shell/` → `owner/`, `pidgsh` →
  `pidowner`, `gsh_*` → `owner_*`); §8.5 se corrige por §6.4 de este documento (alcance
  separado de target); §9 se amplía con la fila de §10.3.
- **Protocolo (`wire-spec.md`, sin escribir):** el `state` con `graph`, la petición de
  nacimiento con el objeto de instructor de §2.1, el evento `step_tree`, y los verbos
  `fail_when`/`abort_when`/`notify_when` en la API del goblin.
- **Sesión de `gash`:** sintaxis de §5.3 (hechos y gramática de reglas), y el parámetro
  `labels` de `run` (§8).

---

## 12. Pendientes

**Abiertos:**
- Gramática exacta de `fail_when`/`abort_when`/`notify_when` y sus hechos (§5.3) — sesión
  de `gash`.
- Detalle de `gobblr`/R más allá del contrato genérico de instructor (§2.4).

**Por verificar:**
- Que `gprocutil.ReadStat` exponga `starttime` y `ppid`.
- Que la cadena de `ppid` resista `sudo`, `env -i` y envoltorios similares.
