# Getting started

## Requisitos

- Linux con **cgroup v2** montado y delegado a tu usuario. Verificá:

  ```bash
  stat -fc %T /sys/fs/cgroup        # debe decir cgroup2fs
  cat /sys/fs/cgroup/cgroup.controllers
  ```

  Necesitás `cpu`, `memory` y `pids` disponibles. `gg discover` camina hasta el
  ancestro escribible sin procesos internos; si no hay delegación, falla con un
  mensaje claro.

- Go 1.22+ y Make.

## Instalación

```bash
git clone git@github.com:entezapallo/goblin.git
cd goblin
make build          # binarios en _build/
make install        # ~/.local/bin + symlinks de grimoire
```

Si `~/.local/bin` no está en tu `PATH`, agregalo.

## Primer job suelto

```bash
summon -- sh -c 'echo hola'
```

`summon` crea el cgroup, el socket y el pipe de muerte, lanza el goblin, hace
`submit` del comando y espera a que el paso cierre. Al terminar, el manifiesto
y el cgroup desaparecen.

Modo detach (el goblin sobrevive a `summon`):

```bash
summon --detach --name demo -- sleep 300
goblinlist              # lo ves vivo
```

## Primer script `.gsh`

```sh
name hola
resources memory=256 cpu=1

define saludo
let
    on_job_exit echo terminó
tel

grun saludo
    echo hola desde gash
nurg
```

Corré con `gash hola.gsh`. El header fija los términos del goblin, `define`
declara un paso y `grun` lo ejecuta. Los cuerpos corren en el `ash` real del
fork.

## Estado en disco

Todo vive bajo `$XDG_RUNTIME_DIR/goblin/` (o `/tmp/goblin/` si no está seteado):

| Archivo | Contenido |
|---|---|
| `<id>.json` | manifiesto del goblin vivo (pid, starttime, cgroup, socket) |
| `notifications.jsonl` | avisos de `notify_when` y eventos `unborn` |
| `<id>.history.jsonl` | pasos cerrados, `step_tree` y logs del goblin |
| `<id>.failures.jsonl` | structs de depuración de workers fallidos |

El historial global está en `~/.local/share/goblin/history.jsonl`.

## Verificar que todo funciona

```bash
go test ./...       # 51 paquetes, incluye tests con cgroupfs real
make test-gg        # create → limit → freeze → kill → sweep
./scripts/gg discover
```
