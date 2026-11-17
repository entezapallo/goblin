# gash — handoff (v2)

Reemplaza al handoff original. Mismo propósito (dejar el estado real del
diseño y del código para quien retome esto), con todo lo decidido en la
sesión de diseño más reciente. Los problemas que seguían sin resolver en
el documento original siguen sin resolver acá — se marcan explícitamente
donde corresponde, no se dan por cerrados sin evidencia.

---

## Contexto: dónde vive gash en el sistema completo

`gash` dejó de ser un componente aislado — hoy es una de las superficies de
entrada a un sistema más grande, con piezas nombradas así:

- **`goblin`**: proceso por script, padre real de los workers, hace `Wait()`
  sobre ellos, vive en su propio árbol de cgroups v2 (`ggroups`, jerarquía
  propia: goblin → paso → worker). Pensado para correr en **background**,
  sin depender de que nadie lo esté observando.
- **`warlock`**: launcher/autómata, uno por usuario. Crea goblins, es quien
  mueve procesos a cgroups, es el backend de `orb`.
- **`orb`**: TUI, solo frontend de `warlock`. Observa y actúa (pausar,
  reanudar, matar) sobre goblins ya existentes — nunca los crea.
- **`gash`**: la superficie de shell. Es **siempre un script, nunca un
  REPL** — corre de principio a fin sin sesión interactiva. Toda
  intervención humana en caliente (ver, pausar, matar un job) pasa por
  `warlock`/`orb`, nunca por `gash` — no existe un "Ctrl+C especial" que
  `gash` deba manejar de forma distinta a cualquier señal que mataría a
  cualquier script.

**Dos garantías distintas, que no hay que confundir:**
1. **Goblin → workers (dura, sin excepciones):** si el goblin muere,
   mueren todos sus workers. Sin huérfanos a nivel de sistema operativo.
2. **gash (instructor) → goblin (blanda, configurable):** el goblin **no**
   necesita que `gash` siga vivo. Si `gash` muere (por cualquier causa:
   señal, crash, cierre de sesión), el goblin queda huérfano y aplica la
   política `on_orphan` del header (default: `keep` + notificar). Esto es
   intencional — el goblin está diseñado para sobrevivir a su instructor.

Nombres confirmados: `gash` (shell), `grun...nurg` (bloque de ejecución),
`def...let...tel` (bloque de declaración de paso — ver más abajo),
`grimoire` (multi-call de applets), `summon`/`banish`/`scry`/`slumber`/
`awaken` (submit/kill/list/pause/resume, CLI aparte, no verbos de `.gsh`).

**Pendiente de nuestro lado:** la sesión goblin sigue usando `gsh` y
`run...nur` en sus documentos (v3/v4/v5), y no se les ha avisado del
cambio de nombre. Tampoco saben que `memory`/`cpu` (no `nmem`/`ncpu`) son
los nombres de parámetro vigentes. Esto es tarea nuestra, pendiente,
postergada a propósito porque su lado también está siendo reformado en
paralelo — no vale la pena sincronizar a medias.

---

## Qué agrega gash sobre ash

Sigue siendo estrictamente un agregado sobre `ash` (el intérprete POSIX
heredado del fork de `go-busybox`, confirmado con ~11.900 líneas ya en el
código descargado, no algo que crecimos nosotros). Verbos nuevos, ya
diseñados con más detalle que en v1:

- **`grun...nurg`** — bloque que **bloquea** hasta que terminan todos sus
  jobs. No hay runs en paralelo entre sí; el paralelismo viene solo de
  `workers=N` dentro de un mismo `grun`. Reemplaza al placeholder de v1
  que corría el cuerpo inline en el intérprete — ahora el cuerpo se manda
  como `submit` real por NDJSON al goblin (nace perezosamente, en el
  primer `grun` del script).
- **`def...let...tel`** — bloque (no builtin de una línea, eso quedó
  descartado). Declara un paso con nombre, sus recursos y sus políticas de
  fallo/aviso. `grun` referencia un `def` ya declarado.
- **`resources`** — builtin de una línea, en el **header**. Techo de
  recursos del **goblin entero** (distinto del techo por paso, que va
  dentro de cada `def`).
- **`notify`** — builtin, en el header. Ver sección propia más abajo.
- **`fail_when` / `abort_when` / `notify_when`** — dentro de un `def`, una
  sola vez cada uno por paso. Ver gramática de condiciones más abajo.
- **`on_job_exit`** (paso terminó con código 0) / **`on_error`** (paso
  falló: código≠0, muerte por señal —incluida la de `banish`—, o OOM).
  Nunca corren los dos para el mismo job. Falta nombre definitivo del
  modificador para que disparen por worker en vez de por paso (candidato
  sin confirmar: `each`).
- **`on_exit kill|wait|keep`** (default `kill`) y **`on_leftover term
  <segundos>`** (default 10s) — sin cambios respecto a versiones previas
  del diseño.

Todo lo anterior son **builtins** — no tocan `tokenizeScript` ni
`compoundStarters`, salvo `grun...nurg` y `def...let...tel`, que sí son
bloques y sí lo hacen.

---

## Qué sabemos de ash.go (sin cambios respecto a v1 — nada de esto se
## investigó todavía)

Archivo: `pkg/applets/ash/ash.go`, ~11.900 líneas, ya así al momento de
descargar el fork — no es código que hayamos agregado nosotros. Intérprete
recursivo descendente escrito a mano, sin AST tradicional.

**Lexer:** `tokenizeScript` (línea ~6472).

**Tabla de cierre de bloque**, `compoundStarters` (línea ~6626):
```go
var compoundStarters = map[string]string{
    "while": "done",
    "until": "done",
    "for":   "done",
    "if":    "fi",
    "case":  "esac",
    "{":     "}",
}
```
Punto de enganche confirmado para los dos bloques nuevos:
```go
"grun": "nurg",
"def":  "tel",   // el cuerpo real abre con "let", ver más abajo
```

**Patrón de las construcciones de bloque existentes**, sin cambios desde
v1: cada una es `func (r *runner) run<X>Script(script string) (int,
bool)`, tokeniza, valida la palabra clave, ubica el cierre con
`findMatchingTerminator`, corta el cuerpo, lo re-serializa con
`tokensToScript` y ejecuta.

**Dos puntos de dispatch, no uno** (líneas ~1040-1080 y ~2749-2765) —
**sigue sin investigarse por qué hay dos.** Con `grun`/`def` como bloques
reales (no builtins), esta pregunta sigue siendo bloqueante: cualquier
bloque nuevo tiene que engancharse en ambos o falla según el camino que
tome el script. Hipótesis sin verificar: podría separar pipeline vs.
no-pipeline. **Pendiente: correr una matriz de prueba** (nivel superior,
dentro de `if`, dentro de función, pipeline, subshell, `$(...)`, `&`) para
entender el alcance real antes de comprometerse a "enganchar en los dos
basta".

**Builtins:** un `switch cmdSpec.args[0]` plano (~línea 3277). `resources`
y `notify` van acá, como un `case` más — no tocan el parser.

**Bug en testdata (sin resolver, mismo que v1):** `testutil.go:200` no
aísla el entorno del proceso hijo en los tests de `export`. Relevante
porque vamos a necesitar tests de variables de entorno para `def`/
`resources`, y conviene arreglarlo antes.

---

## Diseño de `def...let...tel`

```sh
define job1
let
    memory=2G
    cpu=1

    notify_when {gte mempeak(worker) 2G} or {gte filesize(out.txt) 2mb} via email
    abort_when {gt time 2h} then notify via email data=custom-msg
    fail_when extreme then notify via email data=log
tel
```

- `define <job>` abre, `let` inicia el cuerpo (línea propia), `tel` cierra.
- **Sin confirmar del todo:** si el verbo de apertura final es `def` o
  `define` — el ejemplo aprobado usa `define`, pero en el resto de la
  conversación (y en los documentos de la sesión goblin) se sigue
  llamando `def`. Hay que fijar uno.
- Parámetros de recursos del paso: `memory`, `cpu` (confirmados con estos
  nombres — **no** `nmem`/`ncpu`, que aparecieron en un ejemplo pero
  quedaron descartados). El resto de parámetros mencionados en rondas
  previas con la sesión goblin (`workers`, `max_w_per_cpu`, `pids`,
  `swap`, `outputs`, `labels`) siguen vigentes salvo que se diga lo
  contrario.
- **Alcance para el MVP:** lista fija y conocida de parámetros, validada
  por gash con un mapa nombre→parser (no `clave=valor` genérico sin
  validar). Un typo (`memroy=8G`) debe fallar al parsear el script, no
  colarse silencioso hasta el goblin. La extensibilidad real que importa
  es que el código esté armado como un mapa (agregar un `case` es
  barato), no que el lenguaje acepte hoy claves no anticipadas.

---

## Diseño de `grun...nurg`

```sh
grun job1
    script.r --value1 file1 --value2 file2
nurg
```

- Bloquea hasta que terminan todos los workers del paso.
- El cuerpo se manda como `submit` real al goblin (ya nacido o naciendo
  perezosamente en este primer `grun`), no se ejecuta inline en el
  intérprete — el placeholder de v1 queda descartado por completo.
- **Sin confirmar:** si `grun job1` lleva o no dos puntos (`grun job1:`)
  — apareció en un ejemplo, se preguntó explícitamente y nunca se
  confirmó. Tentativamente sin los dos puntos en este documento, dado
  que `:` tiene significado propio en POSIX shell y no vale la pena
  arrastrar la ambigüedad sin necesidad.
- **Sin resolver, heredado de v1:** el roundtrip por `tokensToScript` para
  reconstruir el cuerpo antes de mandarlo — puede no preservar quoting o
  heredocs fielmente. Falta decidir si conviene capturar el texto fuente
  crudo en vez de re-serializar tokens.

---

## Header declarativo

Fijo al nacer el goblin — escribir cualquiera de estos verbos después del
primer `grun` es **error de parseo**.

| Verbo | Qué hace | Default |
|---|---|---|
| `name` | nombre explícito del goblin | nombre del script |
| `resources` | techo de recursos del goblin entero (`memory=`, `cpu=`, ...) | — |
| `notify` | destinos de notificación (ver sección propia) | — |
| `on_orphan kill\|keep\|freeze` | `gash` murió de golpe | `keep` + notify |
| `on_unborn kill\|keep\|freeze` | el goblin no llegó a nacer | `kill` + log |
| `on_headless kill\|freeze\|keep` | el goblin murió con workers vivos | `kill` |

**`orb` NO va en el header** — es una interfaz aparte hacia `warlock`, no
algo que `gash` declare. Esto **contradice** los documentos v4/v5 de la
sesión goblin, que sí lo listan como campo del header. Hay que avisarles
(ver sección de pendientes al final).

**Mecanismo de delimitación header/cuerpo:** sin resolver de forma
definitiva. Propuesta de trabajo: un prepass de líneas antes de tokenizar
con `ash`, header termina en la primera línea que no sea vacía/comentario/
verbo de header, y los verbos de header **no** son builtins globales
(solo se reconocen en esa región), para no tapar binarios o funciones del
usuario con esos nombres. No se recibió un ejemplo concreto que lo
contradiga o confirme todavía.

---

## Gramática de condiciones (`fail_when` / `abort_when` / `notify_when`)

Una sola vez cada verbo por `def`. La expresión completa va en esa única
línea.

- **Predicados:** `all`, `extreme`, `match '<patrón>' in stdout|stderr`
  (patrón opaco para gash — se pasa tal cual al goblin, que lo evalúa con
  regexp/substring nativo, no `awk`/applets externos).
- **Comparadores:** `gte`, `gt`, `lte`, `lt`, `eq`, `neq` — solo sobre
  hechos medibles.
- **Hechos medibles:** `mempeak`, `memmean` (ambos con nivel especificable
  siempre como argumento: worker/paso/goblin), `filesize(ruta)` (ruta
  relativa al **pwd del goblin**), `failed_count`, `oom_count`, `time`.
  `temp` quedó **fuera de alcance** (no hay forma clara de atribuir
  temperatura de hardware a un worker específico).
- **Lógicos:** `and`, `or`, `xor`, `not` (permitido tanto sobre predicados
  como dentro de una comparación agrupada).
- **Agrupación:** `{}` **siempre** obligatorias, incluso con una sola
  condición sin combinar con nada más. Sin precedencia implícita de
  operadores — las llaves resuelven toda ambigüedad.
- **`then <acción>`:** disponible en `fail_when` y `abort_when`, **no** en
  `notify_when` (sería redundante, ya es la notificación en sí). Hoy la
  única acción soportada es `notify via <canal> data=<valor>`, con
  `data=` aceptando un único valor (no combinación).

### `extreme`, en detalle

Es un predicado sin argumentos, con una heurística **fija y no
configurable por el usuario** (vive del lado goblin): dispara si, del
total de workers,
- **40% o más fallaron de forma severa** (OOM o muerte por señal), **o**
- **60% o más fallaron**, sin importar la gravedad.

`extreme` es el **default de `fail_when`** cuando no se escribe nada
explícito.

**Sin resolver:** el default real de `fail_when` (más allá de que
`extreme` sea la heurística) — si un usuario no escribe `fail_when` en
absoluto dentro de un `def`, ¿aplica `extreme` automáticamente, o hace
falta escribirlo explícito? Se asume automático pero no está confirmado
en esos términos exactos.

---

## `notify`

Tres formas de argumento, todas acumulables (múltiples líneas `notify`
suman destinos, no se sobreescriben):

```sh
notify email=a@b.c whatsapp=+1245 sms=+1245
notify '{"email":["a@b.c","x@y.z"],"whatsapp":"+1245","sms":["+1245","+6789"]}'
notify noti-info.json
```

- **`clave=valor`:** un valor **escalar** por clave, siempre. No soporta
  arrays de ninguna forma (ni coma-separado ni repitiendo la clave —
  ambas opciones se descartaron explícitamente).
- **JSON inline** (entre comillas simples, JSON estándar por dentro):
  cualquier clave puede ser escalar o array, sin anidamiento más profundo
  que eso.
- **Archivo `.json`:** siempre válido como tercera forma, mismo esquema
  que el JSON inline.
- No hay una lista cerrada de claves — el conjunto (`email`, `whatsapp`,
  `sms`, `type`, lo que sea) se trata parejo: todas aceptan array en JSON,
  todas son escalares en `clave=valor`.

---

## Preguntas abiertas, sin resolver

1. ¿`def` o `define` como palabra final del verbo de apertura del bloque?
2. ¿`grun job1` lleva o no dos puntos al final?
3. Sintaxis/mecanismo exacto de delimitación header/cuerpo — sigue sin un
   ejemplo que lo confirme de punta a punta.
4. Nombre definitivo del modificador para hooks por worker (candidato:
   `each`), y si los `labels` del worker fallido se exponen como
   variables de entorno al hook (no solo en el struct de depuración).
5. `outputs` vs. fallo parcial: si `fail_when` no dispara con el default
   (`extreme`, que no es "fallan todos"), ¿`outputs` se publica por
   worker individual, o solo si el paso entero se marca éxito? Sigue "sin
   comentarios" de nuestro lado.
6. Los dos puntos de dispatch en `ash.go` (~1048 y ~2749) — sin
   investigar por qué existen, bloqueante para confiar en que enganchar
   los bloques nuevos "en los dos" basta.
7. Si el loop del intérprete de `ash` permite vaciar una cola de eventos
   en puntos seguros (entre comandos, durante la espera de un `grun`) —
   hipótesis apoyada en que `ash` ya maneja traps (22 tests de
   `ash-signals`), sin verificar contra el código real.
8. Matriz de dónde funciona un bloque nuevo dentro de `ash` (nivel
   superior, `if`, función, pipeline, subshell, `$(...)`, `&`) — nunca se
   corrió.
9. Bug de `testutil.go:200` (entorno no aislado en tests de `export`) —
   detectado, no arreglado.
10. Migración de un instructor al cgroup del goblin bajo SSH/servidor
    (permisos de `cgroup.procs` cuando el ancestro común es de systemd) —
    anotado como duda para la sesión goblin, sin investigar.

## Pendiente de comunicar a la sesión goblin (a propósito, no ahora)

- Cambio de nombres: `gash`/`grun...nurg`/`def...let...tel` en vez de
  `gsh`/`run...nur`/lo que tuvieran.
- `memory`/`cpu` como nombres de parámetro, no `nmem`/`ncpu`.
- `orb` no va en el header (contradice v4/v5).
- La pregunta de migración a cgroup bajo SSH (punto 10 de arriba).

Se pospone la sincronización porque su lado también está siendo reformado
en paralelo — no vale la pena mandar una nota parcial todavía.
