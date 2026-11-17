# structure — binarios, paquetes y migración (v1)

Complementa a los cinco documentos anteriores (`ggroups-spec.md`, `goblin_mechanism.md`,
`wire-spec.md`, `orb-spec.md`, `warlock-spec.md`). Este cubre lo que no es diseño de
comportamiento sino organización del repo: qué binarios existen, qué paquetes los sostienen,
qué se reusa del árbol actual y qué se borra.

Los diagramas usan bloques ```mermaid```, que Obsidian renderiza de forma nativa sin
plugins adicionales.

## 0. Cómo leer este documento

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Agregado para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |

---

## 1. Los seis binarios

**[DECIDIDO]**

```mermaid
flowchart TB
    subgraph cli["capa CLI / entrada del usuario"]
        gsh["gsh<br/>(shell, uno por script)"]
        summon_cli["summon / banish / scry<br/>slumber / awaken<br/>(applets de grimoire)"]
        warlock_cli["warlock<br/>(suite CLI: start/stop/status/...)"]
        orb_ui["orb<br/>(TUI)"]
    end

    subgraph daemons["procesos de larga vida"]
        warlock_daemon["warlock<br/>(autómata, uno por usuario)"]
        goblin["goblin<br/>(uno por script/instructor)"]
    end

    raven["raven<br/>(ejecución corta, notificación)"]

    gsh -->|birth, run| warlock_daemon
    gsh -->|run, subscribe| goblin
    summon_cli -->|birth| warlock_daemon
    summon_cli -->|run, banish, etc.| goblin
    warlock_cli -->|start/stop| warlock_daemon
    orb_ui -->|subscribe, goblins| warlock_daemon
    warlock_daemon -->|reconcile, sweep| goblin
    goblin -->|ready, report| warlock_daemon
    goblin -->|exec, stdin JSON| raven
    warlock_daemon -->|exec, stdin JSON| raven
```

| Binario | Rol | Documento de referencia |
|---|---|---|
| `grimoire` | multi-call de applets heredados del fork + `summon`/`banish`/`scry`/`slumber`/`awaken` | `goblin_mechanism.md` §6-7 |
| `goblin` | proceso por script/instructor; padre de los workers | `ggroups-spec.md`, `goblin_mechanism.md` |
| `warlock` | autómata (uno por usuario) + suite CLI | `warlock-spec.md` |
| `orb` | TUI, cliente puro de `warlock` | `orb-spec.md` |
| `raven` | protocolo de notificación, ejecución corta | `wire-spec.md` §8 |
| `gsh`/`gash` | shell y su extensión; instructor por defecto | sesión de `gash` (externa) |

---

## 2. Paquetes propuestos

**[ASUMIDO]** (nombres tentativos, no hay código todavía que los contradiga)

```
cmd/
├── grimoire/     → multi-call: applets heredados + summon/banish/scry/slumber/awaken
├── goblin/       → el proceso por script (NUEVO; no es el dispatcher viejo)
├── warlock/      → autómata + suite CLI
├── orb/          → TUI
└── raven/        → protocolo de notificación

pkg/
├── ggroups/      → cgroups v2 (ggroups-spec.md §12)
├── gutils/       → protocolo NDJSON + actor del goblin
├── gprocutil/    → árbol de procesos (+ Ancestors/Subtree nuevos)
├── discover/     → resolución de goblins por placa/manifiesto
├── history/      → historial append-only
├── wire/         → structs del sobre + ambas APIs (wire-spec.md §3)
└── applets/      → árbol heredado y podado del fork (kill, pgrep, ps, ash, ...)
```

**[DECIDIDO]** El repo tiene 0 commits todavía (dato del handoff original) — no hay
historial de git que proteger, así que el rename de `cmd/goblin` (dispatcher viejo) a
`cmd/grimoire`, con un `cmd/goblin` nuevo de contenido distinto en el mismo cambio, se hace
limpio, sin rodeos para preservar `git blame`.

---

## 3. Nombre del módulo Go

**[DECIDIDO]** Se mantiene `goblin` como nombre del módulo, aunque coincida con el nombre
de uno de los seis binarios — mismo patrón que `docker/docker` en el ecosistema Go. Cambiar
el módulo ahora tocaría cada import del árbol por una razón puramente estética, sin
beneficio funcional.

---

## 4. Migración: qué pasa con cada pieza existente

**[DECIDIDO]**

```mermaid
flowchart LR
    subgraph antes["árbol actual"]
        A1["cmd/goblin<br/>(dispatcher de applets)"]
        A2["cmd/homunculi"]
        A3["pkg/gutils<br/>protocolo NDJSON"]
        A4["pkg/gutils<br/>actor: submit/pause/resume/kill/list/death"]
        A5["pkg/gutils<br/>signalWorker, killTree"]
        A6["pkg/gutils<br/>Spawn"]
        A7["pkg/applets/gprocutil<br/>ReadStat/Children/Descendants"]
        A8["pkg/discover"]
        A9["pkg/history"]
        A10["pkg/applets/pstree"]
        A11["5 applets de verbo<br/>goblinsubmit/pause/resume/kill/list"]
    end

    subgraph despues["árbol nuevo"]
        B1["cmd/grimoire"]
        B2["(nada)"]
        B3["pkg/wire<br/>sobre Request/Response/Event"]
        B4["pkg/gutils<br/>máquina de estados nueva<br/>(goblin_mechanism.md §4)"]
        B5["ggroups.Kill / ggroups.Freeze"]
        B6["startstopdaemon.SpawnInner<br/>+ wrapper delgado"]
        B7["pkg/gprocutil<br/>+ Ancestors + Subtree"]
        B8["pkg/discover<br/>adaptado a manifiesto+id"]
        B9["pkg/history<br/>sin cambios"]
        B10["pkg/applets/pstree<br/>sin cambios"]
        B11["summon/banish/scry/slumber/awaken<br/>(goblin_mechanism.md §6-7)"]
    end

    A1 -->|renombre, contenido intacto| B1
    A2 -->|se borra entero| B2
    A3 -->|reuso tal cual| B3
    A4 -->|se reescribe| B4
    A5 -->|se borra, reemplazo funcional| B5
    A6 -->|se adapta| B6
    A7 -->|reuso + extensión| B7
    A8 -->|se adapta| B8
    A9 -->|reuso tal cual| B9
    A10 -->|reuso tal cual| B10
    A11 -->|renombre + reescritura| B11
```

| Pieza actual | Destino | Motivo |
|---|---|---|
| `cmd/goblin` (dispatcher) | renombrado a `cmd/grimoire`, intacto | libera el nombre `goblin` para el proceso nuevo |
| `homunculi` (binario y todo su directorio) | **borrado entero** | su rol se repartió entre `goblin` (padre real, `Wait()`) y `warlock` (guardián, estado) |
| `pkg/gutils` protocolo NDJSON (`Event`, `Call`, `SimpleVerb`, `Serve`) | **reuso tal cual** | es la forma de sobre que `wire-spec.md` terminó formalizando |
| `pkg/gutils` actor (6 verbos: submit/pause/resume/kill/list/death) | **reescrito** | la máquina de estados de `goblin_mechanism.md` §4 (pasos, reglas, hooks) es mucho más rica |
| `pkg/gutils` `signalWorker`/`killTree` | **borrado** | reemplazado por `cgroup.kill`/`cgroup.freeze` |
| `pkg/gutils` `Spawn` | **adaptado** | pasa a ser wrapper delgado sobre `startstopdaemon.SpawnInner` (`goblin_mechanism.md` §10.2) |
| `pkg/applets/gprocutil` | **reuso + extensión** | se agregan `Ancestors`/`Subtree` sobre la misma base (`ReadStat`) |
| `pkg/discover` | **adaptado** | la convención por PPID se reemplaza por manifiesto+id; se conserva la lógica de "confirmar vivo, borrar huérfanos" |
| `pkg/history` | **reuso tal cual** | el patrón append + `Compact` (temp-file + rename) ya sirve para `<id>.history.jsonl` y `pending.jsonl` |
| `pkg/applets/pstree` | **reuso tal cual** | ya consume `gprocutil` sin depender de `gutils` |
| 5 applets de verbo (`goblinsubmit`/...) | **renombrados y reescritos** | pasan a `summon`/`banish`/`scry`/`slumber`/`awaken`, con direccionamiento nuevo (`goblin_mechanism.md` §6) |

**Nuevo, sin equivalente en el árbol actual:** `pkg/ggroups`, `cmd/orb`, `cmd/raven`,
`pkg/wire` (como paquete propio, aunque reusa el formato de `gutils`), `startstopdaemon.SpawnInner`.

---

## 5. Flujo de nacimiento, para verificar que la estructura cierra

**[ASUMIDO]** Diagrama de secuencia del caso más común (`gsh` corre un `.gsh`, primer
`run`), cruzando los cinco documentos, como prueba de que los binarios propuestos alcanzan
para el flujo completo:

```mermaid
sequenceDiagram
    participant gsh
    participant warlock as warlock (autómata)
    participant ggroups as pkg/ggroups
    participant goblin
    participant raven

    gsh->>warlock: birth {owner, header, script_path}
    warlock->>ggroups: crear <id>/, self/, owner/
    warlock->>ggroups: mover gsh a owner/ (si attach=true)
    warlock->>goblin: lanzar dentro de self/ (UseCgroupFD)
    goblin->>goblin: abrir socket, proceso de id, pidfd sobre gsh
    goblin->>warlock: ready
    warlock-->>gsh: {ok:true, data:{goblin_id}}

    gsh->>goblin: run {job, params, labels}
    goblin->>ggroups: crear step-0001/, w-i/
    goblin->>goblin: spawn workers (SpawnInner + Pdeathsig)
    Note over goblin: bloquea hasta cerrar el paso

    alt un worker falla
        goblin->>raven: exec, stdin={kind:"notify_when", ...}
    end

    goblin-->>gsh: {ok:true, data:{step_id, result}}
```

---

## 6. Fixes chicos del handoff original

**[DECIDIDO]**

- **#9** (`listSummary` imprime el PID como carácter Unicode en vez de decimal): **no
  aplica más**. Ese código vivía en el actor viejo de `pkg/gutils`, que se reescribe
  entero (§4). Se resuelve por omisión, al no portar el bug.
- **#10** (bug de aislamiento de entorno en `testutil.go:200`, suite de `ash`): **sin
  relación** con este diseño — pertenece al árbol heredado de `ash`, intacto. Queda como
  pendiente aparte; candidata natural para resolverse en la sesión de `gash`, que ya toca
  ese árbol directamente.

---

## 7. Instalación

**[DECIDIDO]** Deliberadamente mínima, sin depender de `systemd` — coherente con que
`warlock-spec.md` §3 ya hace que el arranque sea autosuficiente (el primer cliente que
necesita a `warlock` lo despierta).

```mermaid
flowchart LR
    make["make install /<br/>script de instalación"] --> build["compila los 5 binarios"]
    build --> symlinks["symlinks de grimoire<br/>(uno por applet del registry,<br/>patrón BusyBox)"]
    symlinks --> path["todo en PATH del usuario"]
```

**[ASUMIDO]** No hay unidad de systemd de usuario, ni auto-arranque al login. El mecanismo
de `warlock-spec.md` §3 (watchdog + `flock` como red de última instancia) ya cubre el caso
de "nada está corriendo todavía".

---

## 8. Pendientes

**Abiertos:** ninguno nuevo — este documento organiza decisiones ya cerradas en los cinco
anteriores, no introduce preguntas de diseño propias.

**Por hacer, no por decidir:** la migración en sí (mover/renombrar/borrar código real) queda
pendiente de ejecución, no de diseño.
