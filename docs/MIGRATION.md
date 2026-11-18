# Migration

Bitácora del refactor desde `go-busybox` hasta el subsistema goblin. Es
histórico, no documentación de uso — para eso está el [README](../README.md) y
la [documentación](.).

Cada fase terminó con `go build ./...` y `go test ./...` verdes.

0. **Checkpoint** — commit `a45d358` antes del refactor.
1. **Higiene** — borrados `pkg/sandbox/`, los 9 `*_wasm.go` y artefactos sueltos; conectados 8 applets huérfanos al registry (51 entradas).
2. **Baseline verde** — `testutil.go` aísla el entorno por subtest; expectativas de `ash` corregidas; `strftime` registrado en goawk; `time` verifica stderr.
3. **Núcleo nuevo** — `scripts/gg`: superset de cgroups v2 en shell POSIX.
4. **Retirar el motor v1** — manifiestos, verbos híbridos, actor sobre `gg`, `cmd/goblin` como daemon efímero, `cmd/grimoire` como dispatcher. Adiós `homunculi` y `setpgid`.
5. **El contrato** — `pkg/contract`: estados, hechos, reglas, hooks y `Effect`s separados del actor.
6. **Nacimiento** — `pkg/summon` + `cmd/summon`.
7. **`gash`** — frontend/prepass sobre `ash`, sin tocar su parser.
8. **Garantía dura** — `cmd/gguard` + pipe de liveness; `TestGoblinSIGKILLKillsWorker`.
9. **Robustez** — `Call` con timeout, cierre limpio de sesión, `gg sweep`.
10. **Observabilidad** — `pkg/notify`, `cmd/raven`, labels, `history.jsonl`, structs de fallo.
11. **Gramática de reglas** — `contract.Expr` con `Eval(Facts)`, lógicos prefijos, agregadores, `filesize`.
12. **`orb`** — TUI estilo `btm` con layout configurable.
13. **Identidad y `step_tree`** — `starttime`, muestreo del árbol por `cgroup.events`.
14. **`Ancestors`/`Subtree`** — `gprocutil.Identity`, ancestría por identidad.
15. **`Event.Error` tipado** — `Steps`, `Line`, `Message`.
16. **`on_unborn`/`on_headless`** — políticas del header aplicadas en `summon` y `gguard`.
17. **`cpu` fraccionario** — `cpu.max` con quota decimal.
18. **Limpieza de código muerto** — `runRunScript`, `SocketPath`, `MaxWPerCPU`, `swap`, `Command` legacy y build tags WASM.
19. **Fachada final** — README breve, docs por tema, Makefile nuevo, archivado de docs heredados.

Los archivos viejos (`HANDOFF.md`, `plan.md`, `docs/SPEC.md`, `mds/*.md`,
`dev/`) están recuperables desde `d4b4ee5` y la rama `origin/dev`.
