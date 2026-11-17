# ggroups — especificación v1

Borrador de diseño surgido de la sesión de diseño de `goblin`. Cubre la capa `ggroups`
(la abstracción del proyecto sobre cgroups v2), el ciclo de vida de un goblin en
relación con ella, y el contrato de lanzamiento de workers.

## 0. Cómo leer este documento

Cada afirmación relevante lleva una marca:

| Marca | Significado |
|---|---|
| **[DECIDIDO]** | Acordado explícitamente en la sesión. |
| **[ASUMIDO]** | Lo agregué yo para completar el diseño. **Revisar primero estas.** |
| **[ABIERTO]** | Sin decidir. |
| **[SIN VERIFICAR]** | Supuesto sobre el entorno que nadie ha probado todavía. |

Nombres del proyecto: `grimoire` (multi-call de applets), `warlock` (launcher, uno por
usuario), `goblin` (proceso por script), `orb` (TUI, frontend de `warlock`), `gsh`/`gash`
(shell y su extensión de `ash`), `ggroups` (esta capa).

---

## 1. Qué es ggroups

`ggroups` es la capa de `goblin` sobre cgroups v2. Un *ggroup* es un cgroup con nombre que
pertenece a un goblin. Sirve para cuatro cosas:

1. **Matar subárboles completos**, incluidos los procesos que escaparon por double-fork
   y quedaron re-parentados a `init`.
2. **Contratos de recurso**: memoria, CPU, pids y swap, por goblin, por paso y por worker.
3. **Direccionamiento por nombre**: el nombre del ggroup es el id del goblin.
4. **Medición**: pico de memoria, CPU consumido, eventos de OOM.

Paquete propuesto: `pkg/ggroups`.

**Fuera de alcance de la v1:** el controlador `io` (disco), `cpuset` (fijar cores),
cgroups v1 o híbrido, y cualquier sandbox de archivos.

**Reuso.** Ningún applet heredado del fork resuelve cgroups (**[SIN VERIFICAR]**:
`grep -r "sys/fs/cgroup" pkg/applets` lo confirma). Por la política de reuso es mecanismo
nuevo y va en un paquete propio, separado de `procutil`/`gprocutil`, siguiendo el patrón
de `gprocutil`. Toda lectura de `/proc/PID/stat` (por ejemplo `starttime`) debe pasar por
`gprocutil.ReadStat` (**[SIN VERIFICAR]**: confirmar que expone `starttime`).

---

## 2. Requisitos y política estricta

**[DECIDIDO]** Sin cgroups v2 delegados no hay goblin. `warlock` se niega a correr y
`warlock --check` explica qué falta. No hay modo degradado.

| Requisito | Para qué | Versión mínima |
|---|---|---|
| cgroups v2 unificado montado | todo | — |
| Controladores `memory`, `cpu`, `pids` delegados al usuario | límites | — |
| `memory.swap.max` presente | límite de swap | kernel con contabilidad de swap |
| `cgroup.kill` | matar subárboles | 5.14 |
| `cgroup.freeze` | `slumber`/`awaken` | 5.2 |
| `clone3` con `CLONE_INTO_CGROUP` (Go: `SysProcAttr.UseCgroupFD`) | nacer dentro del cgroup, sin ventana | kernel 5.7, Go 1.20 |
| `pidfd_open` | vigilar a `gsh` | 5.3 |
| `memory.peak` | pico de memoria (opcional) | 5.19 |

Si falta `memory.peak`, la métrica de pico se omite. Todo lo demás es obligatorio.

Nota de contexto: RHEL/Rocky 8 traen kernel 4.18 y no cumplen. **[SIN VERIFICAR]** que
la máquina de desarrollo cumpla todo esto.

---

## 3. El goblin como compuesto

**[DECIDIDO]** Un goblin **no es solo un proceso**: es el conjunto de tres partes.

1. **Proceso con metadata:** el proceso `goblin`, su proceso de id y su manifiesto.
2. **ggroup:** su árbol de cgroups.
3. **Script:** el `.gsh` y el `gsh` que lo ejecuta.

Las tres partes pueden morir por separado (ver §9).

### Procesos

| Proceso | Rol | Relación |
|---|---|---|
| `warlock` | launcher, uno por usuario | padre del goblin al lanzarlo |
| `goblin` | padre de los workers; dueño del estado de sus workers; hace `Wait()` | `argv[0] = "goblin:<id>"` |
| proceso de id | solo hace visible el id en `ps`/`pstree` | hijo del goblin, con `PDEATHSIG` |
| `gsh` | el shell del usuario | **no es hijo del goblin**; se mueve a su ggroup |
| workers | procesos de cómputo | hijos directos del goblin, con `PDEATHSIG` |

**[DECIDIDO]** `PDEATHSIG = SIGKILL` en todos los hijos directos del goblin. En Go la señal
se dispara cuando muere el *hilo* que hizo el `fork`, así que todos los lanzamientos deben
hacerse desde una goroutine con `runtime.LockOSThread()` que nunca termina.

**[DECIDIDO]** `gsh` se **mueve** al ggroup (escribiendo su PID en `cgroup.procs`); no cambia
de padre ni de terminal. Consecuencias: el goblin no le hace `Wait()` ni le pone
`PDEATHSIG`; se usa `pidfd` y un mensaje explícito de "terminé bien".

**[ASUMIDO]** El goblin se marca subreaper (`PR_SET_CHILD_SUBREAPER`) para reapear a los
nietos re-parentados, y lanza una goroutine por worker que hace `Wait()` y manda un evento
al actor (el único dueño del estado).

---

## 4. Identidad

**[DECIDIDO]** Formato del id: `<name>-<pidgsh>-<4hex>-<unix>`

- `name`: el verbo `name`, o el nombre del script si no se declara.
- `pidgsh`: PID de `gsh`. Es el único conocido de antemano, porque el id se asigna antes de
  que exista el goblin.
- `4hex`: cuatro dígitos hexadecimales al azar.
- `unix`: segundos Unix al nacer.

**[ASUMIDO]** Reglas:

- `name` se sanea a `[A-Za-z0-9._-]` y se **trunca a 24 caracteres**, porque el id también
  forma parte de la ruta del socket Unix, que tiene un límite de unos 108 bytes.
- La unicidad se garantiza con `mkdir`: `EEXIST` significa colisión y `warlock` reintenta
  con otro sufijo aleatorio.
- El manifiesto guarda la **ruta completa** del ggroup, así `warlock` nunca la recomputa.

### Archivos de un goblin

**[ASUMIDO]** Todos bajo `$XDG_RUNTIME_DIR/goblin/` (reemplazan la convención `<PID>.sock`
de `pkg/discover`):

| Archivo | Contenido |
|---|---|
| `<id>.birth` | configuración de nacimiento, escrita por `warlock` antes de lanzar |
| `<id>.json` | manifiesto final (el `.birth` convertido por `rename`) |
| `<id>.sock` | socket del goblin (la "placa": si responde, está vivo) |
| `<id>.orphaned` | marca creada con `O_EXCL` por quien detecta primero la muerte de `gsh` |
| `<id>.history.jsonl` | log detallado del goblin (el de `pkg/history`) |

**[DECIDIDO]** Un cgroup con `.birth` pero sin manifiesto es un goblin que no llegó a
nacer. El manifiesto se escribe al final, así que si existe, el goblin está listo.

**Campos del manifiesto [ASUMIDO]:** id, name, PID y `starttime` del goblin, PID,
`starttime` y tty de `gsh`, ruta del script, política (`on_orphan`, `on_unborn`,
`on_headless`, `notify`, `orb`), techo de recursos, ruta del ggroup, ruta del socket,
versión del protocolo, hora de nacimiento.

**[ABIERTO]** Con systemd, cerrar la última sesión detiene `user@UID.service` y mata el
árbol, salvo que el usuario tenga *linger* activado. Hay que decidir si los goblins deben
sobrevivir al logout (entonces linger es requisito) o morir con la sesión.

---

## 5. Ruta base

**[DECIDIDO]** Autodetectar, con una ruta configurable como salida de emergencia.

Algoritmo de autodetección:

1. Leer `/proc/self/cgroup` de `warlock` (línea `0::<ruta>`).
2. Subir por la ruta hasta el ancestro llamado `user@<UID>.service`. Ese es el subárbol
   delegado.
3. La base es `/sys/fs/cgroup/<ruta>/goblin/`.
4. Verificar que sea escribible y que `cgroup.controllers` del padre incluya `memory`,
   `cpu` y `pids`.

Si `warlock` no está dentro de un subárbol delegado (típico de un login por SSH, que vive
en `session-N.scope`), es un error con diagnóstico, no un intento de arreglo.

**[ASUMIDO]** Variable de entorno de override: `GOBLIN_CGROUP_BASE`.

**[ASUMIDO]** `warlock` **no** vive dentro de `<base>/goblin/`: permanece en el cgroup donde
arrancó (que debe estar dentro del subárbol delegado). Así `goblin/` solo contiene goblins
y respeta la regla de "sin procesos internos" (§6). El diagrama inicial tenía una hoja
`warlock/`; queda descartada.

---

## 6. Jerarquía

```
<base>/goblin/
└── <id>/                       techo del goblin (límites desde el header)
    ├── self/                   goblin + proceso de id
    ├── shell/                  gsh (movida en el primer run)
    └── step-0001/              un paso (run...nur); límites por paso
        ├── w-1/                worker 1;  memory.high propio
        ├── w-2/
        └── ...
```

Solo hay **un paso activo a la vez**, porque `run...nur` bloquea. **[DECIDIDO]**

### Reglas del kernel que la moldean

- **Sin procesos internos.** Un cgroup con controladores habilitados para sus hijos no
  puede contener procesos. Por eso el goblin vive en `self/`, `gsh` en `shell/` y los
  workers en hojas `w-i/`.
- **Habilitar controladores de arriba hacia abajo**, escribiendo `+memory +cpu +pids` en
  `cgroup.subtree_control` de: `<base>/goblin/`, `<id>/` y `step-N/`. Hay que hacerlo
  **antes** de mover cualquier proceso a ese cgroup: si ya tiene procesos, la escritura
  falla con `EBUSY`.
- **Borrar solo cgroups vacíos** (sin procesos ni hijos). Un zombi cuenta como miembro
  hasta que se reapea.

---

## 7. Recursos

**[DECIDIDO]** Tres niveles: techo del goblin, límite por paso, reparto por worker.

### 7.1 Qué se escribe y dónde

| Nivel | Archivo | Valor | Origen |
|---|---|---|---|
| `<id>/` | `memory.max`, `cpu.max`, `pids.max` | techo del goblin | header |
| `step-N/` | `memory.max` | tope agregado del paso | `def ... memory` |
| `step-N/` | `cpu.max` | cuota agregada del paso | `def ... cpu` |
| `step-N/` | `pids.max` | `pids N` declarado, o **1024 × workers** | `def ... pids` |
| `step-N/` | `memory.swap.max` | `0` (default), tamaño o `max` | `def ... swap` |
| `step-N/` | `memory.oom.group` | `0` | fijo |
| `w-i/` | `memory.high` | `factor × memory.max / N` | automático |

### 7.2 CPU

`cpu N` es una **cuota agregada**: `cpu.max = "N*100000 100000"` (microsegundos de cuota por
periodo de 100 ms). `cpu 2` da `200000 100000`. El reparto entre workers lo hace el
planificador del kernel con pesos iguales (`cpu.weight` por defecto), así que no se escribe
nada por worker. `cpuset` (fijar cores) **[DECIDIDO]** queda fuera de la v1.

### 7.3 Memoria, swap y OOM

- `memory` es un **tope agregado por paso** (**[DECIDIDO]**).
- **[DECIDIDO]** Reparto igual y blando: cada worker tiene su subcgroup con
  `memory.high = factor × memory.max / N`. `memory.high` frena al worker (reclamo directo)
  pero **nunca dispara el OOM**; el muro real es `memory.max` del paso.
- **[DECIDIDO]** `factor` es configurable por paso, con `1.0` por defecto. Se omite
  `memory.high` si el paso no declara memoria o si `workers = 1`.
- **Advertencia:** con `swap = 0`, superar `memory.high` no manda páginas al disco: el
  worker queda frenado en seco. Un worker que legítimamente necesita más que su parte
  (aunque el paso completo esté bajo el tope) se vuelve muy lento. El factor de holgura
  existe para eso.
- **[DECIDIDO]** Swap en `0` por defecto, configurable con el parámetro `swap` de `def`:
  `0`, un tamaño (`4G`) o `max`. Se traduce a `memory.swap.max` del paso.
  **[ASUMIDO]** Solo se escribe cuando el paso declara `memory`; sin `memory`, no se toca.
- **[DECIDIDO]** Si se agota la memoria, el kernel mata al proceso más grande
  (`memory.oom.group = 0`) y los demás workers siguen.
- **Clasificación de fallos por OOM:** se lee el contador `oom_kill` de `memory.events`
  del `w-i/` correspondiente. **[SIN VERIFICAR]** que el contador quede en el cgroup de la
  víctima (debería, pero hay que probarlo en el kernel real).

### 7.4 Admisión de `max_w_per_cpu`

**[DECIDIDO]** Estática, sin cola. Antes de crear el cgroup del paso se comprueba
`workers ≤ ceil(cpu × max_w_per_cpu)`; si no, el `run` falla.

**[ASUMIDO]** Si el paso no declara `cpu`, no se comprueba nada. Si un paso pide más que el
techo del goblin, es un error al lanzar (no se recorta).

### 7.5 Ejemplo trabajado

`def alinear memory 8G cpu 4 workers 4` (sin `swap` ni `pids` declarados):

```
step-0001/memory.max        = 8589934592
step-0001/memory.swap.max   = 0
step-0001/memory.oom.group  = 0
step-0001/cpu.max           = 400000 100000
step-0001/pids.max          = 4096            (1024 × 4)
step-0001/w-1..w-4/memory.high = 2147483648   (8 GiB / 4, factor 1.0)
```

---

## 8. Ciclo de vida

### 8.1 Nacimiento

**[DECIDIDO]** Header declarativo y nacimiento perezoso: el goblin nace en el primer `run`.
Un script sin jobs no crea goblin.

1. `gsh` ya parseó el header. En el primer `run` manda a `warlock` la petición: header
   estructurado (no texto), PID, `starttime` y tty de `gsh`, y ruta absoluta del script.
   `gsh` se bloquea esperando la respuesta.
2. `warlock` valida la base (§5) y asigna el id (§4).
3. `warlock` escribe `<id>.birth` (archivo temporal + `rename`) con la configuración.
4. `warlock` crea `<base>/goblin/<id>/`, habilita los controladores en su
   `cgroup.subtree_control`, aplica el techo y crea `self/` y `shell/`.
5. `warlock` mueve a `gsh` a `shell/`. **[ASUMIDO]** Este paso va *antes* de lanzar el
   goblin, para fallar barato: si la migración da `EACCES`, se limpia (`rmdir`, se borra
   el `.birth`), `gsh` recibe un error claro y el `run` falla. `on_unborn` no aplica, porque
   el goblin nunca se lanzó.
6. `warlock` lanza el goblin dentro de `self/` con `UseCgroupFD`, con
   `argv[0] = "goblin:<id>"`.
7. El goblin abre `<id>.sock`, lanza el proceso de id, abre un `pidfd` sobre `gsh` y
   verifica su `starttime`, y por último escribe el manifiesto (`rename` de `.birth` a
   `.json`).
8. El goblin avisa "listo" a `warlock`; `warlock` responde a `gsh` con el id y el `run`
   continúa.

**[ASUMIDO]** Si el goblin no avisa "listo" en 5 s, `warlock` mata `self/` y `step-*/`
(no `shell/`), registra el fallo y aplica `on_unborn` (default `kill + log`). `gsh` recibe
un error y termina por sí mismo.

### 8.2 Inicio de un paso

1. El goblin recibe el `run` de `gsh` por su socket y valida la admisión (§7.4) y que los
   límites no superen el techo.
2. Crea `step-NNNN/` (cuatro dígitos), habilita controladores y escribe los límites (§7).
3. Prepara el scratch (§11).
4. Por cada worker: crea `w-i/`, escribe `memory.high` si aplica y lanza el proceso con
   `UseCgroupFD`, `Pdeathsig = SIGKILL`, desde el hilo fijado.
5. Una goroutine por worker hace `cmd.Wait()` y manda un evento `exited` al actor.

### 8.3 Fin de paso

1. **Clasificar cada worker** que termina: fallo = código de salida distinto de cero, muerte
   por señal (incluida la de `banish`) o muerte por OOM (**[DECIDIDO]**). El OOM se detecta
   por el contador `oom_kill` de `w-i/memory.events`.
2. **Durante el paso** el goblin evalúa las reglas `abort_when` (incluido el hecho `t`,
   tiempo desde el inicio del paso). Si una se cumple, mata a los workers restantes
   (`cgroup.kill` de cada `w-i/` vivo). Un timeout es solo un caso de `abort_when`.
   **[DECIDIDO]**
3. **Sobrantes.** Cuando todos los workers terminaron, si `step-N/cgroup.events` dice
   `populated 1`, se aplica `on_leftover term <s>` (**[DECIDIDO]**, default 10 s):
   descongelar si estaba congelado, enviar `SIGTERM` a cada PID de `cgroup.procs`, esperar
   hasta `s` segundos a que quede vacío y, si no, `cgroup.kill`. `s = 0` es kill inmediato.
   Siempre se notifica. Un sobrante **no** cuenta como fallo del job.
4. **Reapear zombis** (el goblin es subreaper).
5. **Leer métricas** (§10) y escribirlas en el log.
6. **Resultado del paso.** Regla por defecto **[DECIDIDO]**: falla solo si fallan **todos**
   sus workers. Es configurable con `fail_when` (sintaxis en la sesión de `gash`).
7. **Publicar outputs** si tuvo éxito (§11), o conservar el scratch si falló.
8. **Borrar** `w-i/` y `step-N/` con `rmdir`, con reintento hasta que `populated 0`.
9. **Avisar a `gsh`**, que ejecuta `on_job_exit` (éxito) u `on_error` (fallo) en el
   siguiente punto seguro. Los hooks disparan una vez por paso con un resumen de los N
   workers, o por worker si se pidió la opción. **[DECIDIDO]**

### 8.4 `slumber` / `awaken`

`echo 1 > step-N/cgroup.freeze` y `echo 0 > ...`. El congelamiento es recursivo sobre los
`w-i/`. **[DECIDIDO]** Se aplica **solo al paso activo, nunca a `<id>/` ni a `self/`**,
porque congelaría al propio goblin.

- **[ABIERTO]** ¿El reloj `t` cuenta el tiempo congelado? Recomendación: no.
- **[SIN VERIFICAR]** Que `cgroup.kill` funcione sobre un paso congelado (debería: `SIGKILL`
  atraviesa el freezer). `SIGTERM` a un proceso congelado queda pendiente, por eso
  `on_leftover` descongela antes.

### 8.5 `banish`

- Un paso: `step-N/cgroup.kill = 1`.
- El goblin entero: `<id>/cgroup.kill = 1` (incluye `self/` y `shell/`).

**[ASUMIDO]** `banish` sin argumento apunta al paso activo; con un id, al goblin. Matar un
job por señal cuenta como fallo y dispara `on_error`.

### 8.6 Fin normal del script (`on_exit`)

`gsh` manda "terminé bien" junto con el valor de `on_exit`. Como `run` bloquea, normalmente
no queda ningún worker vivo.

- `kill` (**[DECIDIDO]**, default): se mata lo que quede. Los jobs matados así **no**
  disparan ningún hook.
- **[ASUMIDO]** `wait`: espera a que no queden workers. `keep`: los deja; el goblin, ya sin
  `gsh`, termina cuando el ggroup queda vacío.

Al terminar, el goblin sale y `warlock` borra el ggroup y los archivos del goblin.

### 8.7 Muerte brusca de `gsh` (`on_orphan`)

1. El goblin la detecta por `pidfd`. `warlock` además la detecta al reconciliar (§9).
2. Quien la detecta primero crea `<id>.orphaned` con `O_EXCL` y es el único que notifica.
   **[DECIDIDO]**
3. Se notifica siempre, sin importar la política (§10).
4. Se aplica la política: `kill` (mata el paso activo), `keep` (nada) o `freeze` (congela el
   paso activo). Default `notify + keep`. **[DECIDIDO]**
5. **[ASUMIDO]** Tras un orphan con `keep`, el goblin termina cuando su ggroup queda vacío.
   Los hooks no corren porque `gsh` no existe.

### 8.8 Muerte del goblin (`on_headless`)

Si el goblin muere, `PDEATHSIG` mata al proceso de id y a los workers directos. Los nietos
sobreviven, re-parentados, dentro de su `w-i/`. `warlock` lo detecta (§9) y aplica
`on_headless`: `kill` (`<id>/cgroup.kill`), `freeze` o `keep`. Default `kill`. **[DECIDIDO]**

**Hueco residual conocido:** si el goblin muere **y** `warlock` está caído, esos nietos
siguen vivos hasta que `warlock` vuelva. Solo se cierra al reconciliar.

---

## 9. Reconciliación de `warlock`

**[DECIDIDO]** El reencuentro es "estilo A": `warlock` recorre las placas al arrancar.
**[ABIERTO]** Si además hay un barrido periódico (propuesta: cada 30 s).

Procedimiento:

1. Listar `<base>/goblin/*` y `$XDG_RUNTIME_DIR/goblin/*.{birth,json}`.
2. Para cada id, comprobar las tres partes **con identidad** (PID **y** `starttime`, nunca
   solo el PID, por si se reciclaron):
   - goblin vivo: el PID existe y su `starttime` coincide con el manifiesto.
   - `gsh` vivo: ídem.
   - ggroup: existe y su `populated`.
3. Aplicar la tabla.

| Proceso goblin | ggroup | `gsh` | Estado | Acción |
|---|---|---|---|---|
| vivo | vivo | vivo | sano | reconectar al socket y pedir snapshot |
| vivo | vivo | muerto | huérfano | `on_orphan` (si no existe `<id>.orphaned`) |
| muerto | poblado | cualquiera | sin cabeza | `on_headless` |
| muerto | vacío | cualquiera | terminado | `rmdir`, borrar archivos, cerrar el log |
| vivo | no existe | cualquiera | corrupto | notificar, no matar |
| `.birth` sin `.json` | cualquiera | cualquiera | no nació | `on_unborn` |
| ggroup sin `.birth` ni `.json` | vacío | — | desconocido | `rmdir` |
| ggroup sin `.birth` ni `.json` | poblado | — | desconocido | notificar, **no** matar (**[ASUMIDO]**) |

Un socket que no responde (con timeout) es la placa de un goblin muerto.

---

## 10. Métricas y notificación

### Métricas **[DECIDIDO]**

- **Al cerrar cada paso**, el goblin lee `memory.peak` (si existe), `cpu.stat` (`usage_usec`)
  y `memory.events` del paso y de cada worker, y los guarda en su log. Sobreviven aunque
  `warlock` estuviera caído.
- **Series de tiempo:** las muestrea `warlock` (`memory.current`, `cpu.stat`,
  `pids.current`) y las muestra `orb`. Solo existen mientras `warlock` viva. Los umbrales
  que disparen notificaciones se configuran en `warlock`, no en `gash`.
  **[ABIERTO]** Intervalo de muestreo (propuesta: 2 s).

### Primitivo de notificación

**[DECIDIDO]** Un primitivo aparte, con la terminal como primer destino y el escritorio
como segundo. Los pendientes se guardan y se muestran al abrir `orb`.

**[ASUMIDO]** Diseño: `Emit(evento)` (paquete compartido por goblin y `warlock`, porque el
goblin puede quedarse solo):

1. Agrega el evento al archivo de pendientes `~/.local/share/goblin/pending.jsonl`
   (`O_APPEND`; un "visto" es otra línea; compactación como en `history`).
2. Prueba los destinos en orden. **Terminal:** abre el tty del manifiesto con
   `O_NONBLOCK|O_NOCTTY`, sanea el texto (sin secuencias de escape) y, si el tty ya no
   existe, cae al siguiente. **Escritorio:** `notify-send`.

Después de cerrar sesión no hay D-Bus, por eso el pendiente en archivo es la red de seguridad.
El evento lleva: id, motivo, script, hora y política aplicada.

---

## 11. Contrato de lanzamiento de un worker e IO

**[DECIDIDO]** Cada worker tiene un directorio de trabajo propio, y los outputs se publican
de forma atómica solo si el paso termina bien. El resto es **[ASUMIDO]**.

| Aspecto | Contrato |
|---|---|
| Directorio actual | el de `gsh` al hacer `run` (las rutas relativas del script siguen funcionando) |
| stdin | `/dev/null` |
| stdout / stderr | `~/.local/state/goblin/logs/<id>/step-N/w-i.out` y `.err`; `orb` puede seguirlos |
| Scratch (`GOBLIN_WORKDIR`) | `~/.local/state/goblin/scratch/<id>/step-N/w-i/`; en disco, **no** en tmpfs; override con `GOBLIN_SCRATCH` |
| Variables de entorno | `GOBLIN_ID`, `GOBLIN_STEP`, `GOBLIN_WORKER_INDEX` (1..N), `GOBLIN_WORKER_COUNT`, `GOBLIN_WORKDIR` |
| Outputs | parámetro `outputs` de `def`: rutas de destino |

Se respeta `$XDG_STATE_HOME`. **[ABIERTO]** Retención de logs (propuesta: la de `history`,
un mes).

### Publicación

Solo si el paso tuvo éxito. Por cada output:

- Mismo sistema de archivos: `rename`.
- Distinto: copiar a un nombre temporal dentro del directorio destino y luego `rename`.

La atomicidad es **por archivo, no por conjunto**. Para el conjunto se escribe al final un
marcador `_COMPLETE` en cada directorio destino.

### Conflictos y limpieza

- **Antes de lanzar:** si dos workers o pasos declaran el mismo output, error.
- **Al publicar:** si el destino ya existe, el paso falla, salvo que se pida sobrescribir.
- **Scratch:** se conserva si el paso falló (para inspeccionarlo) y se borra tras publicar si
  tuvo éxito. Opción en `gash` para conservar o borrar siempre.

**Límite conocido:** el goblin no puede impedir que un programa escriba fuera de su
scratch; no hay sandbox. Da herramientas para no pisarse, no una garantía.

---

## 12. API propuesta de `pkg/ggroups`

**[ABIERTO]** Propuesta, no aprobada. Son firmas de diseño, no código probado.

```go
package ggroups

type Limits struct {
    MemoryMax int64   // bytes; 0 = sin límite
    SwapMax   int64   // -1 = max; 0 = sin swap; solo se escribe si MemoryMax > 0
    CPUCores  float64 // 0 = sin límite; cpu.max = cores*100000 100000
    PidsMax   int64   // 0 = sin límite
}

type Base struct{ Path string }

func Detect(override string) (*Base, error) // §5
func (b *Base) Check() Report               // lo usa warlock --check

type Group  struct{ /* <id>/     */ }
type Step   struct{ /* step-N/   */ }
type Worker struct{ /* w-i/      */ }

func (b *Base) NewGoblin(id string, ceiling Limits) (*Group, error)
func (g *Group) Move(pid int, leaf string) error // gsh -> "shell"
func (g *Group) SelfFD() (int, error)            // para UseCgroupFD
func (g *Group) NewStep(n int, lim Limits, workers int, highFactor float64) (*Step, error)
func (s *Step) NewWorker(i int) (*Worker, error)
func (w *Worker) FD() int                        // para UseCgroupFD

// Común a Group, Step y Worker
type Node interface {
    Kill() error
    Freeze(on bool) error                 // Group devuelve error: nunca se congela el goblin
    Procs() ([]int, error)
    Events() (Events, error)              // populated, frozen, oom_kill...
    Stats() (Stats, error)                // memory.current/peak, cpu.stat, pids.current
    WaitEmpty(ctx context.Context) error // inotify sobre cgroup.events
    Remove() error                        // rmdir con reintento
}
```

---

## 13. `warlock --check`

**[DECIDIDO]** Existe un modo de diagnóstico. La lista de comprobaciones es **[ASUMIDA]**.
Debe ejecutarse **desde la terminal donde se usará `gsh`**, porque la migración depende de
dónde vive esa terminal.

1. Versión del kernel (`>= 5.14`).
2. `/sys/fs/cgroup` es cgroup2.
3. La ruta base se detecta y es escribible.
4. `cgroup.controllers` incluye `memory`, `cpu`, `pids` y se pueden habilitar.
5. Existen `memory.swap.max`, `cgroup.kill` y `cgroup.freeze`; `memory.peak` es opcional.
6. `pidfd_open` funciona.
7. Se puede lanzar un proceso dentro de un cgroup de prueba con `UseCgroupFD`.
8. **Migración:** crea un cgroup temporal, mueve el propio PID del `--check` y lo devuelve.
9. Escribir y leer un límite de prueba.
10. `$XDG_RUNTIME_DIR` y la ruta de scratch son escribibles, y el scratch no es tmpfs.

Salida: `OK` o `FALLA` por línea, con una pista de cómo arreglar. Código de salida distinto
de cero si falla algo obligatorio.

---

## 14. Modos de fallo

| Situación | Síntoma | Comportamiento |
|---|---|---|
| Migración de `gsh` sin permiso | `EACCES` al escribir `cgroup.procs` | el goblin no nace; `gsh` recibe error claro |
| Habilitar controladores con procesos dentro | `EBUSY` en `subtree_control` | error de diseño: no debe ocurrir si se respeta §6 |
| Id repetido | `EEXIST` en `mkdir` | `warlock` reintenta con otro sufijo |
| Sin `memory.swap.max` | archivo ausente | falla `--check`; un paso con `swap` declarado da error |
| Sin `cgroup.kill` | kernel < 5.14 | `warlock` se niega a correr |
| Goblin no avisa "listo" | timeout de 5 s | `on_unborn` |
| `rmdir` falla | cgroup con procesos o zombis | reintento hasta `populated 0`; si persiste, notificar |
| `warlock` cae en pleno nacimiento | `.birth` sin `.json` | al volver, el caso "no nació" (§9) |
| Socket de un goblin no responde | timeout | se aplica la tabla de §9 |

---

## 15. Limitaciones conocidas

- **Sin sandbox y mismo UID:** cualquier proceso del usuario puede tocar los ggroups y los
  sockets. Los sockets van con permisos `0600`; se recomienda verificar el UID del otro
  extremo (`SO_PEERCRED`).
- **Hueco residual:** goblin muerto + `warlock` caído (§8.8).
- **Solo cgroups v2 puro:** v1 e híbrido no se soportan.
- **Contenedores:** dentro de un contenedor sin delegación, `--check` falla.
- **Atomicidad de `rename`:** solo dentro de un mismo sistema de archivos; no está garantizada
  en algunos sistemas de red.
- **Cierre de sesión:** ver la pregunta abierta sobre *linger* en §4.
- **`memory.high` con `swap = 0`:** frena en seco (§7.3).
- **Los workers pueden lanzar lo que quieran** (`setsid`, demonios, doble fork): todo sigue
  dentro del ggroup y `on_leftover` lo limpia.

---

## 16. Pendientes

### Para la sesión de `gash`
- Sintaxis del header y de cómo se declara el techo del goblin frente al de cada paso.
- Gramática de `fail_when`/`abort_when` y de sus "hechos" (`oom`, `mem_over`, `t`); el
  significado de `min` quedó sin aclarar.
- Nombre del modificador que dispara los hooks por worker (¿`each`?).
- Parámetros nuevos de `def`: `swap`, `pids`, `outputs`, factor de `memory.high`, y la opción
  de conservar o borrar el scratch.
- Verbos del header: `on_headless`, `on_unborn`, `on_orphan`, `notify`, `orb`, `name`.
- Verbos de paso: `on_leftover`.

### De este diseño (ggroups)
- Barrido periódico de `warlock` y su intervalo.
- ¿`t` cuenta el tiempo congelado?
- Intervalo de muestreo de series de tiempo.
- Retención de logs y del scratch.
- ¿Los goblins sobreviven al logout (linger)?
- Marcador `_COMPLETE`: ¿dónde exactamente y con qué contenido?
- Nombre real de las variables de entorno de override.
- Aprobación formal de la API de `pkg/ggroups`.

### Por verificar en la máquina
- Cgroups v2 delegados con `memory`, `cpu` y `pids`; kernel; `memory.swap.max`.
- Migración de `gsh` desde la terminal real.
- El contador `oom_kill` en el cgroup de la víctima.
- `cgroup.kill` sobre un paso congelado.
- Que `gprocutil.ReadStat` exponga `starttime`.
- Que ningún applet heredado toque cgroups.

---

## Apéndice A. Comprobación manual rápida

Equivale a una versión mínima de `warlock --check`. Ejecútalo desde la terminal real.

```sh
uname -r                                    # >= 5.14
stat -fc %T /sys/fs/cgroup                  # debe decir cgroup2fs
cat /proc/self/cgroup                       # tu cgroup actual

B=/sys/fs/cgroup/user.slice/user-$(id -u).slice/user@$(id -u).service
cat "$B/cgroup.controllers"                 # debe incluir memory cpu pids
cat "$B/cgroup.subtree_control"

# prueba de migración (lo que hará gsh):
mkdir "$B/ggtest"
sleep 300 &
echo $! > "$B/ggtest/cgroup.procs" && echo "migración OK"
echo 1 > "$B/ggtest/cgroup.kill" 2>/dev/null || kill %1   # cgroup.kill necesita 5.14
sleep 1; rmdir "$B/ggtest"
```

Si `mkdir` o el `echo` a `cgroup.procs` dan "Permission denied", esa terminal no vive dentro
del subárbol delegado: es exactamente el caso `EACCES` de §14.
