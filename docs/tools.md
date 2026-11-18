# Tools

## `gash`

Frontend de la DSL. Compila un `.gsh` y lo ejecuta.

```bash
gash pipeline.gsh
gash --ash FILE          # modo interno: corre FILE con ash
```

Lee el header, los `define` y los `grun`, levanta un goblin con `summon` y
envía cada bloque como un paso. Ver [gash.md](gash.md).

## `summon`

Launcher del contract. Crea cgroup, socket y pipe, lanza el goblin, hace
`submit` y espera al cierre del paso.

```bash
summon [--detach] [--name ID] [--memory M] [--cpu C] [--workers N]
       [--orphan keep|kill|freeze] [--unborn …] [--headless …] -- CMD [ARGS...]
```

Sin `--detach` espera y limpia todo. Con `--detach` devuelve el id apenas hace
`submit`; el pipe se omite (`DEATH_FD=-1`) y el goblin vive hasta que terminen
sus pasos. Env: `GOBLIN_GG`, `GOBLIN_BIN`, `GOBLIN_GUARD`.

## `goblin`

Daemon efímero del contrato. No se invoca a mano; lo lanza `summon`.

```
goblin ID SOCKET CGROUP DEATH_FD OWNER_PID [GG_PATH]
```

Env: `GOBLIN_ORPHAN`, `GOBLIN_UNBORN`, `GOBLIN_HEADLESS`, `GOBLIN_NOTIFY`,
`GOBLIN_GUARD`, `GOBLIN_ON_COMPLETE`, `GOBLIN_ON_FAIL`, `GOBLIN_LEFTOVER`.

## `gguard`

Supervisor de liveness por worker. Recibe el fd del pipe (fd 3) y el comando.

```
gguard LIVENESS_FD -- CMD [ARGS...]
```

Al EOF del pipe aplica `GOBLIN_HEADLESS`: `kill`→SIGKILL (default),
`freeze`→SIGSTOP, `keep`→sin señal. Mata al grupo, no solo al líder.

## `raven`

Emisor de notificaciones. Escribe en `$XDG_RUNTIME_DIR/goblin/notifications.jsonl`.

```bash
raven emit email 'job terminado'
```

## `orb`

TUI estilo `btm`. Consulta cgroupfs/manifiestos y muta por socket.

| Tecla | Acción |
|---|---|
| `j` / `k`, ↑/↓ | navegar |
| `p` | pausar workers |
| `r` | reanudar |
| `x` | matar |
| `?` | ayuda |
| `q` | salir |

Layout configurable en `$XDG_CONFIG_HOME/orb/config.json`:

```json
{
  "widgets": ["top", "goblins", "workers", "events"],
  "heights": { "top": 3, "goblins": 6, "workers": 6, "events": 5 }
}
```

No es dinámico: se edita y se reinicia.

## `grimoire`

Dispatcher multi-call de los applets heredados del fork (51 applets: `ash`,
`awk`, `sed`, `grep`, `ps`, `kill`, `pgrep`, …). Invocable por symlink
(`ln -s grimoire sed`) o como `grimoire <applet> [args...]`. También expone
`pstree` y los 5 verbos de contrato (`goblinsubmit`, `goblinpause`,
`goblinresume`, `goblinkill`, `goblinlist`).

## `scripts/gg`

Superset de cgroups v2 en shell POSIX, agnóstico a systemd/init.

```bash
gg discover                 # ancestro escribible sin procesos internos
gg create NAME              # crea el grupo y habilita controladores
gg limit NAME memory=2G cpu=1.5 pids=4
gg freeze NAME / gg thaw NAME
gg kill NAME / gg wait NAME / gg stat NAME / gg list
gg sweep                    # borra grupos huérfanos
```

`GOBLIN_CGROUP` sobreescribe la base. `make test-gg` lo prueba contra
cgroupfs real.
