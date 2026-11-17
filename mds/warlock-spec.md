# warlock — especificación (v1)

Complementa a `ggroups-spec.md` (mecánica del kernel, incluida la reconciliación de §9, que
`warlock` ejecuta), `goblin_mechanism.md` y `wire-spec.md` (protocolo, rol de servidor de
`warlock` en §7). Este documento cubre lo que es propio de `warlock`: su arquitectura
interna, cómo arranca y sobrevive, qué persiste (nada) y su suite CLI.

## 0. Cómo leer este documento

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Agregado para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |

---

## 1. Dos cosas distintas bajo el mismo nombre

**[DECIDIDO]** "Warlock" nombra dos cosas separadas, que conviene no confundir:

1. **El autómata `warlock`** (el engine): el proceso que corre uno por usuario, mantiene la
   coordinación de goblins en memoria mientras vive, y sirve el protocolo de
   `wire-spec.md` §7. Es del que habla el resto de este documento en §2-§5.
2. **La suite CLI `warlock`**: un multi-call al estilo `grimoire`/`go-busybox` — un wrapper
   delgado sobre varios subcomandos/ABIs, uno de los cuales (`start`) lanza al autómata.
   Los demás subcomandos (`wardog`, `apply`, `check`...) son procesos hermanos, no anidados
   dentro del autómata, aunque compartan binario en disco. Ver §6.

---

## 2. Arquitectura interna del autómata

**[DECIDIDO]** Mismo espíritu de diseño que `grimoire`/`go-busybox`: un **engine** central
con **applets** de trabajo alrededor — pero acá "applets" no son binarios invocables, son
unidades de trabajo internas del mismo proceso.

### 2.1 El engine

Una sola goroutine, dueña de todo el estado de coordinación en memoria:

- la lista de goblins conocidos (reconstruida por `reconcile`, actualizada por `sweep` y por
  los reportes entrantes de cada goblin, `wire-spec.md` §6.3),
- la lista de suscriptores activos de `orb` (`wire-spec.md` §4, §7.2),
- el resultado del último `sample` (series de tiempo, para responder de inmediato a un
  `orb` que recién se conecta).

Todo lo demás — conexiones entrantes, el reloj, resultados de trabajo — le habla al engine
por channel. El engine nunca hace I/O pesado ni bloquea esperando una lectura de disco o de
`/proc`; eso es trabajo de los applets (§2.2).

### 2.2 Los applets (unidades de trabajo)

**[DECIDIDO]** Cuatro applets, cada uno con responsabilidad separada en el código, aunque
`sweep` y `sample` puedan compartir una misma pasada de lectura cuando corren en el mismo
tick:

| Applet | Responsabilidad | Disparo |
|---|---|---|
| `reconcile` | reconstruir la lista de goblins desde manifiestos + `cgroup.procs` (`ggroups-spec.md` §9); aplicar `on_headless`/`on_unborn`/`on_orphan` a lo que encuentre en esos estados; buscar procesos `goblin:*` huérfanos de mecanismo vía `pgrep`/`MatchProcs` (`goblin_mechanism.md` §10.3) | una vez, al arrancar (§3) |
| `sweep` | detectar altas/bajas de goblins, anomalías, desde la última pasada | tick periódico del engine |
| `sample` | series de tiempo por goblin/paso, para `orb` (`ggroups-spec.md` §10) | tick periódico del engine; comparte lectura con `sweep` si coincide el intervalo |
| `notify` | invocar `raven` (`wire-spec.md` §8) | por evento (fallo, orphan, headless, umbral cruzado por `sample`) |

### 2.3 El reloj

**[DECIDIDO]** El engine es el **único reloj**: `sweep`/`sample` no llevan timer propio ni
corren como goroutines de larga vida. En cada tick, el engine decide qué applets tocan y los
lanza como tareas puntuales (no como procesos ni goroutines que persisten entre ticks). Si
`sweep` y `sample` comparten intervalo, se lanzan como una sola tarea combinada que hace
ambas lecturas y devuelve dos resultados. `reconcile` corre una única vez, al arrancar
(§3); `notify` se dispara por evento, nunca por tick.

**[ABIERTO]** Intervalo del tick único (candidato: el mismo que el muestreo de series de
tiempo de `ggroups-spec.md` §10, sin fijar ahí tampoco).

---

## 3. Ciclo de vida de arranque

**[DECIDIDO]** Secuencia completa:

1. Un cliente (`gsh`, un applet de verbo, `orb`) intenta conectarse al socket de `warlock`.
   Si no responde, intenta tomar un `flock` sobre un lockfile; quien lo consigue lanza
   `warlock start`. Esto es la **red de última instancia**, no el mecanismo principal — ver
   punto 5.
2. El autómata arranca y corre `reconcile` **una sola vez**, antes de abrir su socket.
3. Recién cuando `reconcile` termina, el autómata abre su socket y empieza a aceptar
   conexiones. **[DECIDIDO]** Ningún cliente ve un estado a medias: no hay una ventana donde
   el socket responda con una lista de goblins incompleta.
4. El autómata lanza a su **watchdog** como proceso separado (`warlock wardog <pid>`, §6),
   pasándole su propio PID. El watchdog espera (vía `pidfd`, el mismo mecanismo usado para
   vigilar al instructor, `ggroups-spec.md` §2) a que el autómata muera.
5. Si el autómata muere — por cualquier causa, incluido `SIGKILL` — el watchdog lo relanza,
   que repite los pasos 2-4 desde cero. Este es el **mecanismo principal** de continuidad.
6. Si el watchdog mismo muere, no pasa nada activamente — no hay vigilancia mutua
   (**[DECIDIDO]**, deliberado: el watchdog es tan simple que la probabilidad de que falle
   él mismo es mucho menor que la del autómata). Recién cuando un cliente falla al conectar
   se dispara de nuevo el paso 1, que relanza tanto al autómata como a un watchdog nuevo.

**[DECIDIDO]** `warlock stop` para ambos procesos a la vez (autómata y watchdog) — pararlo
sin avisarle al watchdog resultaría en un relanzamiento no deseado.

---

## 4. Estado: autómata sin memoria propia

**[DECIDIDO]** `warlock` es un autómata puro: **cero estado propio persistido**. Cada
arranque —sea el primero de la sesión o el número cien tras una caída— reconstruye
exactamente lo mismo desde la misma fuente (manifiestos + `cgroup.procs` de cada goblin,
vía `reconcile`). No hay un archivo de "memoria" de `warlock` entre corridas.

**Consecuencia aceptada explícitamente:** las series de tiempo que `sample` acumula en
memoria (para las gráficas de `orb`, `ggroups-spec.md` §10) se pierden enteras cada vez que
el autómata muere y vuelve a nacer. `orb` empieza a ver datos de nuevo recién en el próximo
tick tras el reinicio, sin arrastrar el tramo anterior. **[DECIDIDO]**: se acepta como
comportamiento normal.

**[DECIDIDO]** Lo único que vive en disco y que `warlock` lee (nunca escribe como "memoria
propia") es:
- el manifiesto y el historial de **cada goblin** (`ggroups-spec.md` §4, `goblin_mechanism.md`
  §9) — no es estado de `warlock`, es del goblin;
- `pending.jsonl` (`ggroups-spec.md` §10) — tampoco es estado de coordinación, es el registro
  de notificaciones, que sobrevive por diseño propio, independiente de si `warlock` está
  vivo o no;
- **un único archivo de configuración**, descrito en §5 — el único que el usuario edita a
  mano y que `warlock` solo lee, nunca escribe.

---

## 5. Configuración

**[DECIDIDO]** Un solo archivo JSON, **solo leído** por el autómata y **solo escrito** por
el usuario (a mano, o validado/aplicado con `warlock apply`, §6). El autómata nunca lo
modifica. **[ASUMIDO]** ruta: `~/.config/goblin/warlock.json`, con override por variable de
entorno (mismo patrón que `GOBLIN_CGROUP_BASE` en `ggroups-spec.md` §5).

**[ABIERTO]** Contenido exacto — candidatos evidentes por lo ya decidido en otros
documentos:
- intervalo del tick único (§2.3),
- umbrales que disparan `notify` desde `sample` (`ggroups-spec.md` §10),
- destinos de notificación por defecto, más allá de terminal/escritorio
  (`ggroups-spec.md` §10),
- tamaño del búfer de suscriptor lento (`wire-spec.md` §4, hoy asumido en 256).

**[ASUMIDO]** Cambios al archivo de configuración **no se recogen en caliente**: hace falta
`warlock apply` (que reinicia el autómata tras validar) para que tomen efecto. Alternativa
para el futuro, no decidida: recarga en caliente sobre `SIGHUP` o similar.

---

## 6. Suite CLI

**[DECIDIDO]** Multi-call, al estilo `grimoire`. Cada subcomando es su propio proceso al
ejecutarse, aunque comparta binario en disco con los demás.

```
warlock start [--foreground]    # lanza el autómata + su wardog. Por defecto, daemoniza.
warlock stop                    # para autómata + wardog, limpio (evita relanzamiento).
warlock restart                 # stop + start.
warlock status                  # ¿vivo? ¿hace cuánto? ¿cuántos goblins?
warlock check                   # diagnóstico de cgroups (ggroups-spec.md §13). Standalone,
                                 #   no requiere que el autómata esté corriendo.
warlock goblins                 # lista goblins (cliente del verbo `goblins`, wire-spec.md §7.1).
warlock logs                    # sigue el propio log del autómata (no el de un goblin).
warlock wardog <pid>            # el proceso watchdog en sí. Lo invoca `start` internamente;
                                 #   nada impide invocarlo a mano.
warlock apply <file.json>       # valida el archivo Y, si es válido, lo aplica (reinicia el
                                 #   autómata para que tome efecto). Un solo paso, no separa
                                 #   validar de aplicar.
```

**[DECIDIDO]** No hay subcomando para editar la configuración campo por campo
(`warlock config set ...`): una sola forma de cambiarla (editar el JSON + `apply`), no dos
caminos redundantes.

**[ASUMIDO]** `warlock status`, si el autómata no está vivo, lo dice explícito ("no está
corriendo; iniciar con `warlock start`") en vez de fallar con un error de conexión crudo —
mismo espíritu que el catálogo de errores de `wire-spec.md` §5, pero a nivel CLI en vez de
protocolo.

---

## 7. Pendientes

**Abiertos:**
- Intervalo del tick único del engine (§2.3).
- Contenido exacto del archivo de configuración (§5): lista cerrada de campos.
- Si la configuración se recarga en caliente alguna vez, o `apply` (con su reinicio) sigue
  siendo la única vía.

**Por verificar:** ninguno nuevo respecto a los ya listados en `ggroups-spec.md` y
`goblin_mechanism.md` — este documento no introduce supuestos nuevos sobre el kernel o el
entorno, solo sobre la forma del propio proceso.
