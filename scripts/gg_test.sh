#!/bin/sh
# Prueba end-to-end de gg contra cgroupfs real.
# Crea una base temporal bajo el ancestro descubierto y la limpia al salir.
set -eu

GG=${GG:-./scripts/gg}

assert_eq() {
    if [ "$1" != "$2" ]; then
        printf 'FALLO: %s != %s\n' "$1" "$2" >&2
        exit 1
    fi
    printf 'ok: %s\n' "$3"
}

BASE_PARENT=$($GG discover)
BASE="$BASE_PARENT/gg-test-$$"
export GOBLIN_CGROUP="$BASE"
RT=$(mktemp -d)
export XDG_RUNTIME_DIR="$RT"

cleanup() {
    for d in "$BASE"/*; do
        [ -d "$d" ] || continue
        $GG kill "${d##*/}" 2>/dev/null || true
        find "$d" -mindepth 1 -type d -depth -exec rmdir {} \; 2>/dev/null || true
        rmdir "$d" 2>/dev/null || true
    done
    rmdir "$BASE" 2>/dev/null || true
    rm -rf "$RT"
}
trap cleanup EXIT

# create: crea el grupo y habilita controladores en el padre
t=$($GG create job1)
[ -d "$t" ] || { echo "FALLO: no se creó $t" >&2; exit 1; }
assert_eq "$(tr ' ' '\n' < "$t/cgroup.controllers" | grep -c '^memory$' || true)" 1 "create: memory disponible"

# list: el grupo aparece
assert_eq "$($GG list)" job1 "list: job1"

# limit: los términos llegan a los archivos del kernel
$GG limit job1 memory=64M cpu=1 pids=4
assert_eq "$(cat "$t/memory.max")" 67108864 "limit: memory"
assert_eq "$(cat "$t/cpu.max")" '100000 100000' "limit: cpu"
assert_eq "$(cat "$t/pids.max")" 4 "limit: pids"

# cpu fraccionario: 1.5 cores → quota 150000
$GG limit job1 cpu=1.5
assert_eq "$(cat "$t/cpu.max")" '150000 100000' "limit: cpu fraccionario"
$GG limit job1 cpu=0.25
assert_eq "$(cat "$t/cpu.max")" '25000 100000' "limit: cpu 0.25"
if $GG limit job1 cpu=abc 2>/dev/null; then
    echo "FALLO: cpu no numérico aceptado" >&2
    exit 1
fi
echo "ok: cpu inválido rechazado"

# enter: un proceso real entra al grupo
sleep 20 &
pid=$!
$GG enter job1 "$pid"
assert_eq "$(cat "$t/cgroup.procs")" "$pid" "enter: pid en cgroup.procs"

# stat: expone métricas
$GG stat job1 | grep -q '^pids.current: 1' || { echo "FALLO: stat" >&2; exit 1; }
echo "ok: stat"

# freeze / thaw: pausa real del grupo
$GG freeze job1
assert_eq "$(awk '/^frozen / {print $2}' "$t/cgroup.events")" 1 "freeze"
$GG thaw job1
assert_eq "$(awk '/^frozen / {print $2}' "$t/cgroup.events")" 0 "thaw"

# kill: mata el subárbol y el grupo queda vacío
$GG kill job1 2>/dev/null
assert_eq "$(cat "$t/cgroup.procs")" '' "kill: cgroup.procs vacío"

# wait: el evento populated=0 se observa
assert_eq "$($GG wait job1)" empty "wait"

# remove: el grupo desaparece
$GG remove job1
[ ! -d "$t" ] || { echo "FALLO: remove" >&2; exit 1; }
echo "ok: remove"

# sweep: borra huérfanos, respeta grupos con manifiesto vivo
$GG create orphan1 >/dev/null
kept=$($GG create kept1)
mkdir -p "$RT/goblin"
printf '{"id":"kept1"}\n' > "$RT/goblin/kept1.json"
$GG create kept1/step-1 >/dev/null
$GG sweep
[ ! -d "$BASE/orphan1" ] || { echo "FALLO: sweep no borró orphan1" >&2; exit 1; }
[ -d "$kept" ] || { echo "FALLO: sweep borró kept1 (tiene manifiesto)" >&2; exit 1; }
echo "ok: sweep"

printf 'gg: todas las pruebas pasaron\n'
