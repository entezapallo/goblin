# wire-spec — protocolo de comunicación entre binarios (v1)

Complementa a `ggroups-spec.md` (mecánica del kernel) y `goblin_mechanism.md` (mecánica
interna del goblin). Este documento cubre **cómo se hablan los binarios entre sí**: formato,
sobre común, las dos APIs (goblin, `warlock`) y el contrato de `raven`.

## 0. Cómo leer este documento

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Agregado para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |

---

## 1. Los binarios y sus roles

Seis binarios. Cuatro de ellos hablan un protocolo estructurado entre sí (el resto de este
documento); `grimoire` y `raven` son casos aparte, explicados en su lugar.

| Binario | Cuántos corren | Rol en la comunicación |
|---|---|---|
| **`warlock`** | uno por usuario | **Servidor** para `gsh`/`gobblr` (nacimiento), `orb` (consulta y control), los applets de verbo (`scry` sin target), y su propia CLI. **Cliente** de cada goblin (reconciliación, reenvío de eventos a `orb`). |
| **`goblin`** | uno por script/sesión | **Servidor** para su instructor (`gsh`/`gobblr`/`summon`) y para los applets de verbo dirigidos a él (`banish -g`, `scry -g`, `slumber`, `awaken`). **Cliente** de `warlock` solo al nacer (avisa "listo") y al reportar eventos para reconciliación. |
| **`orb`** | uno por usuario, cero o más instancias de UI a la vez | **Cliente puro** de `warlock`. Nunca servidor, nunca habla directo con un goblin. |
| **`gsh`** | uno por script en ejecución | **Cliente** de `warlock` (pide nacimiento) y del goblin (manda `run`, recibe eventos para los hooks). |
| **`grimoire`** | multi-call, invocado por symlink | No tiene protocolo propio. Los applets `summon`/`banish`/`scry`/`slumber`/`awaken` son clientes de `warlock` o del goblin, según el caso (§6 de `goblin_mechanism.md`). El resto de los applets heredados no hablan este protocolo en absoluto. |
| **`raven`** | ejecución corta, uno por evento | **Es el protocolo de notificación del sistema**, no un cliente/servidor del protocolo NDJSON de sobre (§2-§7). Toda notificación (`on_orphan`, `on_headless`, `on_unborn`, `notify_when`, umbrales de `warlock`) sale exclusivamente por acá — ningún otro binario habla directo con la terminal, `notify-send` ni escribe `pending.jsonl`. Recibe un struct plano por **stdin** y comunica su resultado por **código de salida** (§8), nunca por stdout. |
| **`gobblr`** (futuro, R) | uno por sesión de R | Mismo rol que `gsh`, del lado del instructor: cliente de `warlock` y del goblin. Detalle fuera de alcance (`goblin_mechanism.md` §2.4). |

**Regla general [DECIDIDO]:** un goblin nunca es cliente de `orb`, ni de otro goblin. Toda
comunicación hacia `orb` pasa por `warlock` (`goblin_mechanism.md` no lo dice explícito, pero
se sigue de que `orb` es "cliente puro de warlock").

---

## 2. Transporte y formato

**[DECIDIDO]** NDJSON sobre socket Unix (`$XDG_RUNTIME_DIR/goblin/*.sock`). Una línea de
texto, un objeto JSON, un `\n`.

- **Lectura de líneas:** `bufio.Reader.ReadString('\n')`, no `bufio.Scanner` — sin techo de
  64 KB. **[DECIDIDO]** Cualquier error de lectura o de parseo de una línea (JSON inválido,
  línea sin `\n` antes de EOF) se registra en el log del proceso que la recibió (el goblin en
  su `<id>.history.jsonl`, `warlock` en su propio log) antes de cortar esa conexión.
  **[DECIDIDO]**
- **Seguridad del socket [DECIDIDO]:** permisos `0600` **y** verificación de
  `SO_PEERCRED` en cada conexión aceptada, comparando contra el UID del propio proceso
  servidor. Una conexión de otro UID se rechaza antes de leer nada.

---

## 3. El sobre

**[DECIDIDO]** Tres formas distintas, no una sola con campos opcionales. El cliente decide
cuál deserializar mirando qué campos trae el objeto (`verb` → `Request`; `ok` → `Response`;
`event` → `Event`).

```go
type Request struct {
    V    int             `json:"v"`
    ID   string          `json:"id"`             // obligatorio siempre, sin excepción
    Verb string          `json:"verb"`
    Data json.RawMessage `json:"data,omitempty"`
}

type Response struct {
    V    int             `json:"v"`
    ID   string          `json:"id"`              // copiado del Request
    OK   bool            `json:"ok"`
    Err  *Error          `json:"err,omitempty"`
    Data json.RawMessage `json:"data,omitempty"`
}

type Event struct {
    V     int             `json:"v"`
    ID    string          `json:"id"`              // ver §3.1
    Event string          `json:"event"`
    Seq   uint64          `json:"seq"`
    Data  json.RawMessage `json:"data,omitempty"`
}

type Error struct {
    Code    string `json:"code"`     // catálogo cerrado, §5
    Message string `json:"message"`  // texto legible, puede cambiar libremente
}
```

### 3.1 `id` obligatorio en todo, incluidos los eventos

**[DECIDIDO]** Todo mensaje lleva `id`, sin excepción — incluso los eventos empujados por una
suscripción, que no responden a un `Request` puntual. Para esos, `id` es un identificador
único del evento en sí (**[ASUMIDO]**: un ULID generado por el emisor), útil para
desduplicar si el cliente reconecta y por accidente vuelve a recibir el mismo evento en el
snapshot de resincronización.

### 3.2 Versión

**[DECIDIDO]** `hello` **no** es obligatorio como primer mensaje de una conexión (opción b):
cualquier servidor valida `v` en **cada** mensaje entrante y responde
`{"code": "version_mismatch", ...}` si no coincide, sin exigir un handshake previo. `hello`
existe como verbo normal, disponible por si un cliente quiere preguntar versión y
capacidades explícitamente antes de mandar nada más (útil para `gobblr`, que puede querer
verificar antes de intentar cualquier otra cosa).

---

## 4. Suscripción: snapshot + secuencia

**[DECIDIDO]** Mecanismo único, usado tanto en goblin↔warlock como en warlock↔orb.

```
→ {"v":1,"id":"a1","verb":"subscribe","data":{"since":0}}
← {"v":1,"id":"a1","ok":true,"data":{"snapshot":{...},"seq":41}}
← {"v":1,"id":"b7f...","event":"worker_exit","seq":42,"data":{...}}
← {"v":1,"id":"c22...","event":"worker_exit","seq":43,"data":{...}}
```

- El `Request` de `subscribe` recibe una `Response` con el snapshot completo y el `seq`
  desde el cual arrancan los eventos siguientes.
- Cada `Event` posterior trae `seq` estrictamente creciente. Si el cliente detecta un hueco
  (recibió `seq=43` sin haber visto `42`), pide un `subscribe` nuevo (nuevo snapshot).
- **Suscriptor lento [DECIDIDO]:** cada conexión de suscripción tiene un búfer de salida
  acotado (**[ASUMIDO]**: últimos 256 eventos). Si se llena porque el cliente no lee a
  tiempo, el servidor **corta la conexión**. El cliente debe reconectar y volver a pedir
  snapshot. El servidor nunca bloquea esperando a un suscriptor.

---

## 5. Catálogo de errores

**[DECIDIDO]** Código + mensaje, catálogo corto y cerrado — no un código por cada situación
imaginable. **[ASUMIDO]** lista inicial, se extiende solo cuando un cliente real necesite
distinguir un caso nuevo:

| Code | Cuándo |
|---|---|
| `version_mismatch` | `v` del mensaje no coincide con lo que el servidor soporta |
| `bad_request` | JSON válido pero `verb`/`data` no reconocidos o mal formados |
| `not_found` | el goblin, paso o worker referenciado no existe |
| `wrong_state` | la operación no es válida en el estado actual (p. ej. `run` sobre un goblin `dying`) |
| `permission_denied` | `SO_PEERCRED` no coincide, o la operación exige otro dueño |
| `internal` | error inesperado del servidor; se loguea con detalle server-side |

---

## 6. API del goblin

Servidor: el proceso goblin. Clientes: su instructor (`gsh`/`gobblr`/`summon`), `warlock`
(solo al reconciliar), y los applets de verbo dirigidos a él con `-g`.

### 6.1 Verbos (`Request` → `Response`)

| Verbo | Quién lo llama | `data` de entrada | `data` de salida |
|---|---|---|---|
| `hello` | cualquiera | `{}` | `{version, capabilities}` |
| `birth_config` | el instructor, en el primer `run` (ver nota) | objeto de instructor (`goblin_mechanism.md` §2.1) + header | — (la respuesta real llega por `warlock`, ver §6.3) |
| `state` | `scry`, `orb` (vía warlock), instructor | `{graph: bool}` | manifiesto + estado actual + pasos + workers; árbol de ancestría si `graph=true` |
| `run` | el instructor | `{job, params, labels, hooks}` | `{step_id, result}` — **bloquea** hasta que el paso cierra (`goblin_mechanism.md` §4.2) |
| `slumber` | `slumber`, instructor | `{step: N?}` | `{ok}` |
| `awaken` | `awaken`, instructor | `{step: N?}` | `{ok}` |
| `banish` | `banish`, instructor | `{scope: "step"\|"worker"\|"all", step?, worker?}` | `{ok}` |
| `subscribe` | instructor (hooks), `orb` indirecto, `warlock` (reconciliación) | `{since: seq}` | snapshot + stream, §4 |
| `owner_done` | el instructor, al terminar bien | `{on_exit}` | `{ok}` |
| `shutdown` | `warlock` únicamente (`on_headless`, `on_unborn` de emergencia) | `{}` | `{ok}` — dispara `dying` |

**Nota sobre `birth_config`:** en la práctica, el nacimiento pasa por `warlock` primero
(`goblin_mechanism.md` §8.1 de `ggroups-spec.md`: el instructor le pide el goblin a
`warlock`, no al goblin directamente — el goblin todavía no existe). Este verbo está listado
por completitud del catálogo de la API del goblin, pero **no se llama desde afuera**; es
interno, entre `warlock` y el goblin recién lanzado, en el paso 7 del nacimiento
(`ggroups-spec.md` §8.1).

### 6.2 Eventos (`Event`, vía `subscribe`)

| Evento | Cuándo |
|---|---|
| `worker_spawned` | un worker nace dentro de un paso |
| `worker_exit` | un worker termina (código, señal, o `oom` — `goblin_mechanism.md` §8) |
| `step_started` / `step_closed` | transición de paso (`goblin_mechanism.md` §4.2) |
| `abort_triggered` | una regla `abort_when` se cumplió |
| `notify_triggered` | una regla `notify_when` se cumplió (además de llamar a `raven`, §7) |
| `orphaned` | el instructor murió de golpe y se detectó (`ggroups-spec.md` §8.7) |
| `goblin_dying` | el goblin entró en `dying` |

### 6.3 Mensajes que el goblin manda como cliente (hacia `warlock`)

El goblin abre una conexión saliente hacia el socket de `warlock` en dos momentos:

| Verbo (goblin → warlock) | Cuándo |
|---|---|
| `ready` | terminó de nacer (paso 8 de `ggroups-spec.md` §8.1) |
| `report` | reenvía eventos relevantes para que `warlock` los tenga sin tener que suscribirse activamente (orphan, headless propio si el goblin detecta su propia anomalía, cierre de paso con métricas) |

---

## 7. API de `warlock`

Servidor: `warlock`. Clientes: `gsh`/`gobblr` (nacimiento), `orb`, los applets de verbo sin
target (`scry`), la CLI de `warlock` mismo (que se conecta a su propia instancia corriendo).

### 7.1 Verbos

| Verbo | Quién lo llama | `data` de entrada | `data` de salida |
|---|---|---|---|
| `hello` | cualquiera | `{}` | `{version, capabilities}` |
| `birth` | el instructor, en el primer `run` | `{owner: {...}, header: {...}, script_path}` | `{goblin_id}` — bloquea hasta el paso 8 del nacimiento, o error `internal` si el timeout de 5s se cumple (`ggroups-spec.md` §8.1) |
| `goblins` | `scry` sin target, `orb` | `{}` | lista de goblins conocidos (id, name, estado, instructor) |
| `subscribe` | `orb` | `{since: seq}` | snapshot + stream, §4 |
| `pending` | `orb`, `raven ack` indirecto | `{}` | lista de notificaciones pendientes (`ggroups-spec.md` §10) |
| `ack` | `orb` | `{event_id}` | `{ok}` — marca una notificación como vista |
| `banish` | `banish -g` cuando el goblin no responde directo (fallback) | `{goblin_id, scope, ...}` | `{ok}` — aplica `cgroup.kill` aunque el proceso goblin ya no exista |
| `check` | la CLI `warlock --check` | `{}` | resultado de diagnóstico (`ggroups-spec.md` §13) |

### 7.2 Eventos

| Evento | Cuándo |
|---|---|
| `goblin_born` / `goblin_dead` | altas y bajas, tras reconciliación o en vivo |
| `goblin_headless` / `goblin_orphaned` | políticas aplicadas (`ggroups-spec.md` §8.7-8.8) |
| (reenvío) cualquier evento de §6.2 de algún goblin suscrito | `warlock` actúa de relay hacia `orb`, agregando `goblin_id` al `data` |

**[ASUMIDO]** `warlock` no reenvía automáticamente **todos** los eventos de todos los
goblins a todo suscriptor de `orb` sin filtrar — un usuario con 50 goblins activos no
necesita cada `worker_spawned` de cada uno. `subscribe` acepta un filtro opcional
(`{goblin_id?, kinds?}`); sin filtro, trae solo los eventos "de resumen"
(`goblin_born/dead/headless/orphaned`, `step_started/closed`), no el detalle de cada worker.
**[ABIERTO]**: lista exacta de qué cuenta como "resumen" por defecto.

---

## 8. `raven`: el protocolo de notificación

**[DECIDIDO]** `raven` es, en sí mismo, **el protocolo de notificación del proyecto** —
un protocolo aparte del NDJSON de sobre de §2-§7, deliberadamente más simple porque no hay
conversación entre dos partes: solo una entrada, un efecto, y un resultado.

Cualquier binario que necesite avisar algo al usuario (goblin: `on_orphan`, sobrantes de un
paso; `warlock`: `on_headless`, `on_unborn`, umbrales de uso; el motor de reglas del
goblin: `notify_when`, `goblin_mechanism.md` §5.2) lo hace **exclusivamente** invocando
`raven`, nunca escribiendo a una terminal, llamando a `notify-send` o tocando
`pending.jsonl` por su cuenta. Esto es lo que permite que la notificación funcione aunque
`warlock` esté caído (`ggroups-spec.md` §10): el emisor solo necesita poder ejecutar un
binario, no una conexión NDJSON viva a nadie.

`raven` es un binario de ejecución corta que el goblin o `warlock` invocan con `exec`,
pasándole un único JSON por **stdin** — sin el sobre `Request`/`Response` de §3, porque no
hay conversación, solo una entrada y un código de salida.

```json
{
  "v": 1,
  "id": "01J...",
  "ts": "2026-09-20T16:40:12Z",
  "source": "goblin",
  "kind": "orphan",
  "goblin_id": "alinear-4821-a3f9-1758380000",
  "script": "/home/u/alinear.gsh",
  "message": "gsh murió; política aplicada: keep",
  "policy": "keep",
  "tty": "/dev/pts/3",
  "sinks": ["terminal", "desktop"]
}
```

**Respuesta:** por código de salida, no por stdout.

| Código | Significado |
|---|---|
| `0` | evento guardado en `pending.jsonl` (aunque algún destino, como `notify-send`, haya fallado) |
| `1` | no se pudo guardar el evento (falla grave: disco lleno, permisos) |
| `2` | entrada inválida (JSON mal formado, `v` no soportada) |

**Cómo se entera `orb` [DECIDIDO, opción c del punto anterior]:** `orb` vigila
`pending.jsonl` directamente con `inotify`, sin pasar por `warlock` ni por `raven`. Marca
"visto" llamando a `raven ack <event_id>` (que agrega una línea `{"t":"ack",...}` al mismo
archivo) — o, equivalentemente, al verbo `ack` de `warlock` (§7.1), que hace lo mismo pero
centralizado. **[ABIERTO]**: si ambos caminos (llamar a `raven ack` directo, o pasar por
`warlock`) deben coexistir, o solo uno es el oficial.

---

## 9. Ejemplo de punta a punta

Para fijar cómo se cruzan los tres documentos (`ggroups-spec.md`, `goblin_mechanism.md`,
este), el flujo de un `summon` sin `--detach`:

```
$ summon --memory 4G --cpu 2 --workers 1 -- ./modelo.R

summon (instructor=terminal)
  → warlock:  {"verb":"birth", "data":{"owner":{"kind":"cli","attach":true,...}, "header":{...}}}
  ← warlock:  {"ok":true, "data":{"goblin_id":"modelo-..."}}
                                  (warlock: crea ggroup, mueve la terminal si attach,
                                   lanza el proceso goblin con UseCgroupFD, ggroups-spec §8.1)
  (el goblin recién nacido avisa "ready" a warlock por su cuenta, §6.3 de este documento)

summon (ya con goblin_id)
  → goblin:   {"verb":"run", "data":{"job":"modelo","params":{...}}}
                                  (bloquea: goblin_mechanism.md §4.2, paso pending→active)
  ← goblin:   {"ok":true, "data":{"step_id":1,"result":{"status":"success"}}}

summon
  → goblin:   {"verb":"owner_done", "data":{"on_exit":"kill"}}
  ← goblin:   {"ok":true}
                                  (goblin: dying → dead; warlock borra ggroup y archivos)

$ echo $?   # código de salida del job dentro de modelo.R
```

---

## 10. Pendientes

**Abiertos:**
- Qué cuenta como evento "de resumen" por defecto en `subscribe` de `warlock` sin filtro
  (§7.2).
- Si `raven ack` directo y el verbo `ack` de `warlock` coexisten o hay uno oficial (§8).
- Tamaño exacto del búfer de suscriptor lento (asumido: 256 eventos, §4).

**Para la sesión de `gash`:** ninguno nuevo — este documento no agrega superficie de sintaxis
de `.gsh`, solo fija cómo `gsh` habla por debajo con `warlock` y el goblin.
