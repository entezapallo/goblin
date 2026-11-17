#!/usr/bin/env bash
# =============================================================================
# cleanup-hardfork.sh
# Poda post-hardfork: saca lo que era "prueba de que forkear valía la pena"
# (ya cruzada) y deja solo lo que protege código que TODAVÍA se toca.
#
# Por defecto corre en modo DRY-RUN (solo muestra qué haría).
# Usar --execute para aplicar de verdad.
# =============================================================================
set -euo pipefail

DRY_RUN=1
[[ "${1:-}" == "--execute" ]] && DRY_RUN=0

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$REPO_ROOT"

BUSYBOX_TESTDATA="pkg/testdata/busybox-testsuite"
INTEGRATION_DIR="pkg/integration"
ARCHIVE_REF="archive/pre-hardfork-cleanup-$(date +%Y%m%d)"

# fuzz tests que SÍ se quedan: gash (ash) siempre, más los que tocan
# datos externos hostiles por diseño (parsers de archivos comprimidos/tar).
KEEP_FUZZ_DIRS=("ash" "tar" "gzip" "gunzip")

log() { printf '%s\n' "$*"; }
run() {
  if [[ $DRY_RUN -eq 1 ]]; then
    printf '[dry-run] %s\n' "$*"
  else
    eval "$@"
  fi
}

# -----------------------------------------------------------------------
log "### 0. Chequeo de estado git ############################################"
if git rev-parse --git-dir >/dev/null 2>&1; then
  if git rev-parse HEAD >/dev/null 2>&1; then
    log "-- hay commits, creo un ref de respaldo antes de borrar: $ARCHIVE_REF"
    run "git tag '$ARCHIVE_REF'"
  else
    log "-- repo sin commits todavía; no hace falta ref de respaldo"
  fi
else
  log "-- no es un repo git; no hay respaldo automático, andá con cuidado"
fi
echo

# -----------------------------------------------------------------------
log "### 1. Baseline: tests ANTES de tocar nada ##############################"
run "go test ./... -count=1 > /tmp/hardfork_before.log 2>&1 || true"
log "-- guardado en /tmp/hardfork_before.log"
echo

# -----------------------------------------------------------------------
log "### 2. Borrar fixtures del busybox-testsuite (ya cumplió su propósito) ##"
if [[ -d "$BUSYBOX_TESTDATA" ]]; then
  run "rm -rf '$BUSYBOX_TESTDATA'"
else
  log "-- $BUSYBOX_TESTDATA no existe, skip"
fi
echo

# -----------------------------------------------------------------------
log "### 3. Borrar los tests que dependían de ese testdata ###################"
for f in busybox_testsuite_test.go busybox_compare_test.go; do
  target="$INTEGRATION_DIR/$f"
  if [[ -f "$target" ]]; then
    run "rm -f '$target'"
  else
    log "-- $target no existe, skip"
  fi
done
echo

# -----------------------------------------------------------------------
log "### 4. Borrar *_fuzz_test.go de applets congelados ######################"
log "-- se conservan: ${KEEP_FUZZ_DIRS[*]}"
while IFS= read -r -d '' f; do
  dir="$(basename "$(dirname "$f")")"
  skip=0
  for keep in "${KEEP_FUZZ_DIRS[@]}"; do
    [[ "$dir" == "$keep" ]] && skip=1 && break
  done
  if [[ $skip -eq 1 ]]; then
    log "-- conservado ($dir): $f"
  else
    run "rm -f '$f'"
  fi
done < <(find pkg/applets -name '*_fuzz_test.go' -print0)
echo

# -----------------------------------------------------------------------
log "### 5. Tests DESPUÉS de la poda ##########################################"
run "go test ./... -count=1 > /tmp/hardfork_after.log 2>&1 || true"
log "-- guardado en /tmp/hardfork_after.log"
echo

# -----------------------------------------------------------------------
log "### 6. Diff antes/después ################################################"
if [[ $DRY_RUN -eq 0 ]]; then
  diff -u /tmp/hardfork_before.log /tmp/hardfork_after.log || true
else
  log "[dry-run] (el diff real solo corre con --execute)"
fi
echo

log "### FIN ###################################################################"
if [[ $DRY_RUN -eq 1 ]]; then
  log "Esto fue un dry-run. Para aplicar de verdad: $0 --execute"
fi
