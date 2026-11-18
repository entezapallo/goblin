# La DSL `gash`

`gash` es un frontend/prepass sobre `ash`: parsea el header y los bloques, y
delega los cuerpos de shell al intérprete `ash` real del fork (349/349 en su
suite). No modifica el parser de `ash`.

## Estructura de un `.gsh`

```sh
name alineamiento              # header
resources memory=2G cpu=2 workers=2
notify email=yo@example.com
on_orphan keep
on_unborn kill
on_headless kill

define align                   # declaración de un paso
let
    memory=512
    cpu=1
    workers=2
    labels part=1
    abort_when {and {gte failed_count 2} {match 'CUDA out of memory' in stderr}}
    fail_when {extreme}
    notify_when {not {eq oom_count 0}} then notify via email data=log
    on_job_exit echo OK
    on_error echo FAIL
    on_exit kill
    on_leftover term 10
tel

echo inicio                    # shell normal
grun align part=2              # ejecuta el paso
    bwa mem ref.fq reads.fq > out.sam
nurg
echo fin
```

## Header

Fijo al nacer el goblin. Escribirlo después del primer `grun` es error de
parseo.

| Verbo | Significado | Default |
|---|---|---|
| `name` | id del goblin | generado |
| `resources` | techo del goblin entero (`memory=`, `cpu=`, `workers=`) | — |
| `notify` | canales de notificación (`email=x sms=y`) | — |
| `on_orphan` | muere el instructor: `keep\|kill\|freeze` | `keep` + notify |
| `on_unborn` | falla el nacimiento: `keep\|kill\|freeze` | `kill` + log |
| `on_headless` | muere el goblin con workers: `keep\|kill\|freeze` | `kill` |

`cpu` acepta decimales: `cpu=1.5` → `cpu.max = "150000 100000"`.

## `define … let … tel`

Declara un paso con nombre. Acepta `def` y `define`. Cuerpo:

- **Recursos:** `memory=`, `cpu=`, `workers=`, `pids=`.
- **`labels k=v`**: se pasan al struct de fallo y como `GOBLIN_LABEL_<K>` en los hooks.
- **Reglas:** `fail_when`, `abort_when`, `notify_when` (una de cada, opcional).
- **`on_job_exit <cmd>`** / **`on_error <cmd>`**: hooks al cerrar con éxito/fallo.
- **`on_exit kill|wait|keep`** y **`on_leftover term <segundos>`**.

## `grun <paso> [k=v …] … nurg`

Bloque que bloquea hasta que terminan sus workers. El cuerpo se envía como un
`submit` real al goblin; no corre inline. Las labels de `grun` sobreescriben las
del `define`. Un `.gsh` sostiene **un solo goblin**: cada `grun` es un
`step-N` con sus propios recursos, reglas y hooks.

## Gramática de reglas

Sin precedencia implícita: `{}` es la única agrupación y los lógicos son
**prefijos**.

- **Hechos:** `failed_count`, `oom_count`, `time`, `mempeak(level)`,
  `memmean(level)`, `filesize(ruta)`, `min`/`max` sobre picos de workers,
  `oom`, `all`, `extreme`, `match '<pat>' in stdout|stderr`.
- **Comparadores:** `gte`, `gt`, `lte`, `lt`, `eq`, `neq` con sufijos
  `K/M/G` (memoria) y `s/m/h` (tiempo).
- **Lógicos:** `and`, `or`, `xor`, `not`.
- **Acción:** `then notify via <canal> data=<valor>`; `data=log` inyecta el
  `stderr_tail` del paso.

Ejemplo:

```sh
abort_when {and {gte failed_count 2} {match 'out of memory' in stderr}}
fail_when {or {extreme} {gt time 2h}}
notify_when {not {eq oom_count 0}} then notify via email data=log
```

`extreme` es una heurística fija: dispara si ≥40% de los workers fallan de
forma severa (OOM/señal) o ≥60% fallan por cualquier causa.
