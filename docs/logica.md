# Lógica del contract

## Qué es un contract

Un contract es un conjunto de procesos con **identidad, límites, estados y
eventos observables**. No es un proceso anónimo que se lanza y se pierde. El
modelo viene de los contracts de illumos; la frontera de contención en Linux es
cgroups v2 (la abstracción de procesos bien hecha del kernel, no el hierarchy de
una distro).

La jerarquía es **goblin → paso → worker**:

```
<base>/goblin/<id>/          goblin
└── step-1/                  paso
    ├── w-1/                 worker (proceso real)
    └── w-2/
```

Cada proceso del subárbol vive en una hoja `w-i`. `cgroup.procs` de esas hojas
es la fuente autoritativa de pertenencia.

## Las dos garantías

**Dura:** si el goblin muere, mueren sus workers. No depende de código vivo del
goblin: cada worker corre detrás de `gguard`, que sostiene el extremo de lectura
de un pipe cuyo write end solo tiene el goblin. Si el goblin muere de golpe
(`SIGKILL`, OOM, crash), el kernel cierra el fd, `gguard` ve EOF y aplica
`on_headless` al grupo del worker. No se usa `Pdeathsig`: con el runtime de Go
dispara cuando muere el hilo creador, no el proceso.

**Blanda:** el goblin puede sobrevivir a su instructor. Es intencional — el
goblin está diseñado para ser un daemon efímero, no para morir con la terminal.
Al morir el instructor, el pipe de muerte da EOF y el goblin aplica
`on_orphan`.

## Estados

Goblin: `alive → running → (orphaned) → dying → dead`.

Paso: `pending → starting → active → draining → closing → closed{success|failed|aborted}`.

Worker: `spawning → running → exited | killed | oom_killed`.

## Identidad

Un proceso se identifica por `(pid, starttime)`, nunca solo por PID (se
recicla). `gprocutil.ReadStat` expone `starttime` (campo 22 de
`/proc/PID/stat`), y el manifiesto guarda la identidad del goblin y del
instructor. `discover.Resolve` y `Ancestors` comparan por identidad.

## Direccionamiento

Sin socket fijo por PPID. Los verbos resuelven en este orden:

1. `-g <id>` explícito
2. manifiesto en `$XDG_RUNTIME_DIR/goblin/<id>.json`
3. `GOBLIN_ID`
4. ancestría por `/proc` (comparando identidad)
5. error con candidatos

Las **consultas** (`goblinlist`, `orb`) leen manifiestos y cgroupfs directo.
Las **mutaciones** (`submit`, `pause`, `resume`, `kill`) van por socket NDJSON
al goblin, para que la política medie cada acción. Es el modelo híbrido.

## Decisiones arquitecturales

- **Prepass sobre `ash`, no fork del parser:** los dos puntos de dispatch
  (~1048 y ~2774) y la tabla `compoundStarters` son delicados; tocarlos
  ponía en riesgo los 349/349 tests. `gash` parsea `grun…nurg` afuera y los
  cuerpos corren en el `ash` real.
- **Sin `warlock`:** no hay daemon persistente que posea el estado. El goblin
  es el daemon (nace con el script, muere con él). El estado son archivos:
  cgroupfs + manifiestos JSON.
- **`raven` es una CLI, no un servicio:** escribe `notifications.jsonl`.
- **Garantía dura por pipe, no por `Pdeathsig`:** ver arriba.

## Observabilidad

- `notifications.jsonl` — avisos de `notify_when` y eventos `unborn`.
- `history.jsonl` — pasos cerrados, global con retención de 30 días.
- `<id>.history.jsonl` — log del goblin, incluye el `step_tree` (muestreo del
  árbol por cambio de `cgroup.events:populated`, con resguardo de 5 min, hasta
  500 procesos, con `argv0`, `ppid` y `first_seen`/`last_seen`).
- `<id>.failures.jsonl` — struct de fallo: labels, exit, `stderr_tail` (50 KB)
  y el `status.json` opcional que el job dejó en `GOBLIN_WORKDIR`.
