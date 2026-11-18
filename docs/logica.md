# Lógica del contract

## Casos de uso

`goblin` existe para jobs computacionales pesados donde el modelo de "lanzar y
olvidar" no alcanza: el trabajo tarda horas, consume recursos caros, se reparte
en muchos procesos y necesitás saber qué pasó aunque nadie esté mirando.

### Bioinformática

Un pipeline NGS típico encadena `bwa` → `samtools` → `bcftools` sobre cientos
de muestras. Acá:

- con `workers=N` se paralelizan particiones dentro de un paso;
- `abort_when {match 'CUDA out of memory' in stderr}` corta el paso en caliente
  en vez de esperar a que todos los workers fracasen;
- si el worker que lanzó `bwa` muere, `gprocutil.Ancestors`/`Subtree` y el
  `step_tree` dejan constancia de qué hijos corrieron, para saber qué partición
  no convergió;
- `labels part=1,rows=200-400` viajan al struct de fallo y a los hooks.

### Entrenamiento de ML

Un entrenamiento con varias GPUs: `memory=48G cpu=8`, `nur` cierra el paso, y
`on_headless freeze` permite **pausar de verdad** (`cgroup.freeze`) para
liberar la GPU sin perder el estado, y reanudar después. Sin contract, pausar
un entrenamiento es matarlo y reiniciar desde un checkpoint.

### Cómputo científico y geoespacial

Simulaciones largas con `notify_when {gt time 2h}` avisan por
`notifications.jsonl` sin bloquear, y `fail_when {extreme}` cierra el paso con
fallo si más del 60% de los workers fallan (o 40% de forma severa). El patrón
`grun…nurg` encadena pasos dependientes sin un orquestador externo.

### Por qué un contract y no un cron o un `nohup`

| Necesidad | Con `nohup`/`&` | Con goblin |
|---|---|---|
| Saber si el job terminó bien | código de salida suelto | estados + `history.jsonl` |
| Matar todo el trabajo | `kill` al líder, hijos huérfanos | `cgroup.kill` del subárbol |
| Pausar sin perder trabajo | no existe | `cgroup.freeze` real |
| Saber qué proceso falló y por qué | logs mezclados | `failures.jsonl` + `stderr_tail` + `status.json` |
| Sobrevivir al cierre de la terminal | sí, pero sin control | `on_orphan` explícito |
| Límites de memoria/CPU por job | `ulimit` por proceso | `memory.max`/`cpu.max` jerárquicos |

En corto: el contract convierte un conjunto de procesos en una unidad con
**identidad, límites, observabilidad y una garantía de muerte**, que es justo lo
que falta cuando el job es caro y nadie lo está mirando.

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
