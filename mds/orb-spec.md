# orb — especificación del TUI (v1)

Complementa a `wire-spec.md` (protocolo), `ggroups-spec.md` y `goblin_mechanism.md`
(mecánica de fondo). Este documento cubre la interfaz de `orb`: layout, paneles, acciones y
de dónde sale cada dato.

## 0. Cómo leer este documento

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Agregado para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |

---

## 1. Filosofía

**[DECIDIDO]** `orb` es **cliente puro de `warlock`** (`wire-spec.md` §1): nunca decide
nada por sí mismo, solo pide, muestra, y manda comandos que `warlock` o el goblin
correspondiente ejecutan. Nunca habla directo con un goblin.

**[DECIDIDO]** Toda la pantalla se alimenta de una sola conexión `subscribe` a `warlock`
(`wire-spec.md` §4 y §7.1): snapshot al abrir, eventos en vivo después. Sin polling activo
propio — `orb` vive esperando el próximo evento, como `btm`.

**Excepción [DECIDIDO]:** el panel de pendientes (§4) no pasa por `warlock` para leer;
vigila `pending.jsonl` directo con `inotify` (`wire-spec.md` §8, `ggroups-spec.md` §10).

---

## 2. Layout general

Cinco paneles fijos más un panel de árbol aparte (§6). Navegación por teclado, sin mouse
obligatorio — modelo similar a `btm`.

```
┌─ goblins ──────────────────────────────┬─ modelo-R-9f21 ──────────────────────┐
│ ● alinear-4821-a3f9   running  step 2/3│ owner: gsh (pid 4821) · script:      │
│ ○ modelo-R-9f21       alive            │   /home/u/alinear.gsh                │
│ ▲ backup-cc03         headless  ⚠      │                                      │
│ ● descarga-11a4       running          │ ┌─ mem (paso) ──────┐┌─ cpu ──────┐  │
│                                         │ │ ▂▃▅▇▇▆▅▃▂▁ 6.1/8G ││ ▁▃▅▇▅▃▁2.1c│  │
│                                         │ └────────────────────┘└────────────┘│
│                                         │ [graph] [slumber] [awaken] [banish] │
│                                         │ [ack]                                │
├─ workers (todos los goblins) ─────────────────────────────────────────────────┤
│ goblin              step  worker  pid   estado    mem   cpu  labels          │
│ alinear-4821-a3f9  0002    w-1   5011  running   1.6G  48%  part=1 rows=0-200│
│ alinear-4821-a3f9  0002    w-2   5012  running   2.0G  51%  part=2 200-400  │
│ alinear-4821-a3f9  0002    w-3   5013  exited(1)  —     —   part=3 400-600 ⚠│
│ alinear-4821-a3f9  0002    w-4   5014  running   1.8G  44%  part=4 600-800  │
│ descarga-11a4      0001    w-1   6210  running   0.4G   9%  url=archivo3.gz │
├─ pendientes (3) ───────────────────────┬─ log: alinear/w-3 stderr (tail) ───┤
│ ⚠ backup-cc03 quedó sin cabeza          │ Warning: 340 NA values found       │
│ ⚠ alinear/w-3 falló (oom)               │ Error: model did not converge      │
│ ℹ modelo-R-9f21 terminó bien            │                                     │
└─────────────────────────────────────────┴─────────────────────────────────────┘
 [ctrl+f] buscar goblin   [tab] panel   [↑↓] navegar   [enter] filtrar workers
```

---

## 3. Panel de goblins (izquierda)

**[DECIDIDO]** Lista lo que devuelve `goblins` de `warlock` (`wire-spec.md` §7.1). Símbolo
de estado por fila:

| Símbolo | Estado (`goblin_mechanism.md` §4.1) |
|---|---|
| `●` | `running` |
| `○` | `alive` (sin paso activo) |
| `▲` | `headless` u `orphaned` (con `⚠`) |

**[ASUMIDO]** `unborn` no aparece en la lista (dura como mucho 5s, `ggroups-spec.md` §8.1);
`dying`/`dead` desaparecen de la lista tras un breve parpadeo o de inmediato, a definir.

Se actualiza solo con `goblin_born`/`goblin_dead`/`goblin_headless`/`goblin_orphaned`
(`wire-spec.md` §7.2). Seleccionar una fila decide qué muestran el panel de detalle y filtra
(opcionalmente, ver `enter`) el panel de workers.

**Búsqueda [DECIDIDO]:** `ctrl+f` abre un campo de búsqueda por `name`/`id` sobre esta lista.

---

## 4. Panel de detalle (arriba-derecha)

**[DECIDIDO]** Título = nombre del goblin seleccionado (no implícito por selección sola).
Contenido: dueño (`owner: <kind> (pid ...)`), script, paso activo, y dos gráficas chicas
(memoria y CPU del paso activo), alimentadas por el muestreo periódico que hace `warlock`
(`ggroups-spec.md` §10) y reenvía como parte del estado.

**Botones de acción [DECIDIDO]:** `[graph] [slumber] [awaken] [banish] [ack]`, visibles
dentro del panel, navegables con flechas + enter (o clic si hay mouse) — no son solo
atajos invisibles del pie.

| Botón | Efecto |
|---|---|
| `graph` | abre el panel de árbol de ancestría (§6), pantalla aparte |
| `slumber` | `slumber` sobre el paso activo del goblin (`goblin_mechanism.md` §7), con confirmación |
| `awaken` | `awaken` sobre el paso activo, con confirmación |
| `banish` | `banish`, con confirmación y menú de alcance (paso / worker / todo) |
| `ack` | marca como vistas **todas** las notificaciones pendientes de este goblin (distinto del `a` puntual del panel de pendientes, §5) |

**[ASUMIDO]** Toda acción destructiva (`slumber`, `banish`) exige una confirmación explícita
antes de mandar el `Request` correspondiente — coherente con que estas acciones matan o
congelan cómputo real, no solo lo muestran.

Si el goblin seleccionado no tiene paso activo (`alive`), las gráficas se reemplazan por un
mensaje `sin paso activo`, y `slumber`/`awaken` quedan deshabilitados. **[ASUMIDO]**

---

## 5. Panel de workers (medio, ancho completo)

**[DECIDIDO]** Global, no solo del goblin seleccionado: una fila por worker de **todos**
los goblins `running`, con columna `goblin` explícita — así se ve de un vistazo qué goblin
posee cada worker.

Columnas: `goblin`, `step`, `worker`, `pid`, `estado`, `mem`, `cpu`, `labels`
(`goblin_mechanism.md` §8). Un worker fallido se marca con `⚠`.

**Filtro [DECIDIDO]:** `enter` sobre una fila del panel de goblins alterna entre "todos los
goblins" y "solo el seleccionado" en este panel.

Seleccionar un worker fallido actualiza automáticamente el panel de log (§7) para mostrar
su `stderr_tail`.

Fuente de datos: eventos `worker_spawned`/`worker_exit` de cualquier goblin, reenviados por
`warlock` con `goblin_id` agregado (`wire-spec.md` §7.2). **[ASUMIDO]** El campo `mem`/`cpu`
en vivo por worker requiere que `warlock` muestree también a nivel worker, no solo a nivel
paso — `ggroups-spec.md` §10 habla de series de tiempo por paso; extender a por-worker
queda **[ABIERTO]**.

---

## 6. Panel de árbol de ancestría (pantalla aparte, vía `[graph]`)

**[DECIDIDO]** No comparte espacio con las gráficas de mem/cpu del panel de detalle — es
una pantalla modal más grande, propia.

```
┌─ ancestry: alinear-4821-a3f9 ──────────────────────────────────── [esc cerrar] ─┐
│                                                                                    │
│  gsh (pid 4821)  instructor                                                       │
│  └── goblin (pid 4830)                                                            │
│      ├── idproc (pid 4831)                                                        │
│      └── step-0002  active                                                        │
│          ├── w-1 (pid 5011)  running          part=1 rows=0-200                   │
│          ├── w-2 (pid 5012)  running          part=2 rows=200-400                 │
│          ├── w-3 (pid 5013)  exited(1) ⚠       part=3 rows=400-600                 │
│          │   ├── bwa (pid 5013.1)      [visto 14:02:10 - 14:02:41]                │
│          │   └── samtools (pid 5013.2) [visto 14:02:38 - 14:02:41]                │
│          └── w-4 (pid 5014)  running          part=4 rows=600-800                 │
│                                                                                    │
│  muestreo por evento (populated) · último: hace 12s · resguardo idle: 5min        │
└────────────────────────────────────────────────────────────────────────────────┘
 [↑↓] navegar nodo   [enter] ver stderr_tail / detalle del nodo   [esc] volver
```

**Fuente de datos [DECIDIDO]:**
- Estructura viva: `state --graph` (`wire-spec.md` §6.1, `goblin_mechanism.md` §1).
- Nodos históricos (`bwa`, `samtools`, con su rango `[visto...]`): el registro `step_tree`
  guardado en `<id>.history.jsonl` al cerrar cada paso (`goblin_mechanism.md` §9).

`enter` sobre un worker fallido muestra su `stderr_tail` (`goblin_mechanism.md` §8) en un
panel chico superpuesto, sin salir de la vista de árbol.

---

## 7. Panel de pendientes (abajo-izquierda)

**[DECIDIDO]** Lee `pending.jsonl` directo, vigilado con `inotify` — no pasa por `warlock`
para esto (`wire-spec.md` §8). `a` sobre una notificación puntual llama `raven ack` para
marcarla vista (distinto del `[ack]` global del panel de detalle, §4).

Símbolos: `⚠` para orphan/headless/fallo, `ℹ` para informativo (por ejemplo, terminó bien).
**[ASUMIDO]**

---

## 8. Panel de log (abajo-derecha)

**[DECIDIDO]** Sigue en vivo el stdout/stderr de lo que esté seleccionado (worker o
goblin), vía `scry --logs` (`goblin_mechanism.md` §7), como un `tail -f` embebido. El título
antepone `goblin/worker` (por ejemplo `alinear/w-3`) para no confundir workers del mismo
nombre en goblins distintos.

---

## 9. Navegación general (pie de pantalla)

**[DECIDIDO]**

| Tecla | Efecto |
|---|---|
| `ctrl+f` | buscar goblin (§3) |
| `tab` | cambiar de panel activo |
| `↑↓` | navegar dentro del panel activo |
| `enter` | según panel: filtrar workers al goblin seleccionado (§5), activar botón (§4), ver detalle de nodo (§6) |
| `esc` | cerrar panel modal (árbol, confirmaciones) |
| `q` | salir |

---

## 10. Diferencias con `btm` que vale la pena marcar

- `btm` mide procesos del sistema entero sin pedir permiso a nadie; `orb` confirma antes de
  cualquier acción destructiva, porque sus acciones matan cómputo real, no solo lo muestran.
- `btm` no tiene "pendientes" — es específico de `goblin`, porque un goblin puede sobrevivir
  sin que nadie lo esté mirando.
- `btm` refresca con polling a intervalo fijo; `orb` no hace polling propio — todo llega por
  el stream de `subscribe`, tan rápido como el evento real.

---

## 11. Pendientes

**Abiertos:**
- Si `mem`/`cpu` por worker en vivo (§5) requiere extender el muestreo de `warlock` a nivel
  worker, más allá de lo ya definido a nivel paso (`ggroups-spec.md` §10).
- Comportamiento exacto de `dying`/`dead` en la lista de goblins (§3): parpadeo breve o
  desaparición inmediata.
- Qué cuenta como evento "de resumen" para el filtro por defecto de `subscribe`
  (`wire-spec.md` §7.2) sigue determinando qué tan en vivo se ve todo esto sin pedir
  detalle explícito — comparte el mismo pendiente que `wire-spec.md` §10.
