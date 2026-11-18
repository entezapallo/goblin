# Using

## Pipelines con `gash`

```sh
name pipeline
resources memory=4G cpu=2 workers=2
notify email=yo@example.com

def map
let
    memory=1G
    workers=2
    labels stage=map
    fail_when {extreme}
    on_error echo 'map falló' >&2
tel

def reduce
let
    memory=512
    abort_when {gt time 30m}
tel

grun map
    for f in data/*.fq; do bwa mem ref.fa "$f" > "${f}.sam"; done
nurg

grun reduce
    samtools merge out.bam data/*.sam
nurg
```

Cada `grun` es un paso del mismo goblin. Si un worker falla y dispara
`abort_when`, los demás workers del paso se matan y el paso cierra `aborted`.

## Gestionar en vivo con `orb`

Levantá `orb` en otra terminal. Muestra goblins, workers, sparklines de
CPU/memoria y eventos. Con `p`/`r`/`x` pausás, reanudás o matás los workers
del goblin seleccionado. No necesita permisos especiales: solo lee archivos y
habla por el socket del manifiesto.

También podés usar los applets sueltos desde `grimoire`:

```bash
goblinlist
goblinsubmit -g demo -- sleep 60
goblinkill -g demo <pid>
```

## Notificaciones

`notify_when` y el evento `unborn` escriben en
`$XDG_RUNTIME_DIR/goblin/notifications.jsonl`:

```json
{"ts":"2026-11-17T12:00:00Z","channel":"email","data":"alerta"}
```

Los canales son datos, no credenciales: `raven` no habla por red. Si querés
enviar un mail de verdad, enganchá un consumer de ese archivo.

## Historial y debug

- `~/.local/share/goblin/history.jsonl` — resumen global de pasos cerrados.
- `<id>.history.jsonl` — log del goblin; incluye el `step_tree` con el árbol
  de procesos muestreado por evento y cada 5 min en idle.
- `<id>.failures.jsonl` — un struct por worker fallido:

  ```json
  {
    "worker": "step-1/w-1",
    "labels": {"part": "1"},
    "exit": {"code": 1, "signal": null, "oom": false},
    "stderr_tail": ["Error: modelo no convergió"],
    "status": {"reason": "muchos NA"}
  }
  ```

  El `status` aparece si el job escribió `status.json` en su
  `GOBLIN_WORKDIR` antes de salir.

## Config del TUI

`$XDG_CONFIG_HOME/orb/config.json`:

```json
{
  "widgets": ["top", "goblins", "workers", "events"],
  "heights": { "top": 3, "goblins": 6, "workers": 6, "events": 5 }
}
```

## Troubleshooting

**`gg discover` dice que no hay subárbol escribible.** Tu cgroup no está
delegado. Verificá `cat /sys/fs/cgroup/cgroup.controllers` y que tu
`user@<uid>.service` tenga `cpu memory pids` en `cgroup.subtree_control`. En
contenedores puede requerir mount de cgroup2 con `nsdelegate`.

**`goblinkill` dice "goblin no encontrado".** El manifiesto ya se borró (el
goblin murió) o no seteaste `-g`/`GOBLIN_ID` y no hay ancestría. Corré
`goblinlist` para ver candidatos.

**Quedó un cgroup huérfano.** `summon` llama a `gg sweep` al arrancar, que
borra grupos sin manifiesto y sin procesos. Podés correrlo a mano.

**Un worker quedó vivo tras matar el goblin.** Revisá `on_headless`: con
`keep` el worker sobrevive por diseño; con `freeze` queda en estado `T`.
El default es `kill`.
