#!/usr/bin/env bash
# =============================================================================
# Auditoría de peso/relevancia de pkg/applets/ash/testdata frente al resto del repo
# =============================================================================
set -uo pipefail

ASH_TD="pkg/applets/ash/testdata"
ASH_TEST="$ASH_TD/ash_test"
FUZZ="$ASH_TD/fuzz"

echo "### 0. Contexto git #####################################################"
git rev-parse --show-toplevel 2>/dev/null || echo "(no es un repo git)"
echo

echo "### 1. Tamaño: ash vs resto del repo ####################################"
echo "-- du total del repo:"
du -sh . 2>/dev/null
echo "-- du de ash/testdata (bloques reales):"
du -sh "$ASH_TD" 2>/dev/null
echo "-- du de ash/testdata (tamaño aparente, sin redondeo a bloque):"
du -sh --apparent-size "$ASH_TD" 2>/dev/null
echo "-- du del resto del repo (excluyendo ash/testdata):"
du -sh --exclude="$ASH_TD" . 2>/dev/null
echo "-- tamaño REAL en .git (comprimido):"
git count-objects -vH 2>/dev/null
echo

echo "### 2. Desglose por subcarpeta dentro de testdata #######################"
du -sh "$ASH_TD"/* 2>/dev/null | sort -rh
echo

echo "### 3. Cuántos archivos por subcarpeta en ash_test ######################"
find "$ASH_TEST" -maxdepth 1 -type d | while read -r d; do
  echo "$(find "$d" -type f | wc -l)  $d"
done | sort -rn
echo

echo "### 4. .tests.fail vs .tests que pasan ##################################"
echo -n "total .tests.fail: "
find "$ASH_TEST" -name '*.tests.fail' | wc -l
echo -n "total .tests ok:   "
find "$ASH_TEST" -name '*.tests' ! -name '*.fail' | wc -l
echo "-- .fail por subcarpeta (para ver dónde está el peso muerto):"
find "$ASH_TEST" -name '*.tests.fail' |
  awk -F/ '{print $(NF-1)}' | sort | uniq -c | sort -rn
echo

echo "### 5. Corpus de fuzz: tamaño, cantidad y ESTRUCTURA ####################"
du -sh "$FUZZ" 2>/dev/null
find "$FUZZ" -type f 2>/dev/null | wc -l
echo "-- estructura esperada por Go: testdata/fuzz/FuzzXxx/<hash>"
find "$FUZZ" -maxdepth 2 -type d 2>/dev/null
echo "-- ¿hay funciones Fuzz en el paquete que consuman esto?"
grep -rn "func Fuzz" pkg/applets/ash/ 2>/dev/null || echo "(ninguna función Fuzz)"
echo

echo "### 6. Archivos sueltos raros en ash_test ###############################"
find "$ASH_TEST" -maxdepth 1 -type f 2>/dev/null
echo

echo "### 7. ¿Trackeado, ignorado o LFS? ######################################"
echo -n "archivos trackeados en testdata: "
git ls-files "$ASH_TD" 2>/dev/null | wc -l
echo "-- ¿está ignorado?"
git check-ignore -v "$FUZZ" 2>/dev/null || echo "(no ignorado)"
git check-ignore -v "$ASH_TEST" 2>/dev/null || true
echo "-- .gitattributes relevante (LFS, diff, eol):"
[ -f .gitattributes ] && grep -E 'testdata|fuzz|lfs|filter' .gitattributes || echo "(sin .gitattributes o sin coincidencias)"
echo

echo "### 8. Top 20 archivos más pesados ######################################"
find "$ASH_TD" -type f -printf '%s %p\n' 2>/dev/null | sort -rn | head -20
echo

echo "### 9. Symlinks y duplicados exactos ####################################"
echo "-- symlinks:"
find "$ASH_TD" -type l 2>/dev/null || true
echo "-- duplicados (mismo md5):"
find "$ASH_TD" -type f -exec md5sum {} + 2>/dev/null |
  sort | uniq -w32 -dD | head -50
echo

echo "### 10. ¿Quién menciona / necesita esto? ################################"
echo "-- referencias en código Go:"
grep -rn "testdata/ash_test\|testdata/fuzz" pkg/applets/ash/*.go 2>/dev/null || echo "(sin referencias directas)"
echo "-- referencias en todo el repo (por si hay scripts, CI, etc.):"
grep -rn "testdata/ash_test\|testdata/fuzz" --include='*.go' --include='*.sh' --include='*.yml' --include='*.yaml' --include='Makefile' . 2>/dev/null | grep -v "^./$ASH_TD" | head -50
echo

echo "### 11. Procedencia / historia ##########################################"
echo "-- commits que tocaron testdata (últimos):"
git log --oneline -n 30 -- "$ASH_TD" 2>/dev/null
echo "-- cuándo se añadió el corpus de fuzz:"
git log --diff-filter=A --oneline -- "$FUZZ" 2>/dev/null | head
echo "-- cuándo se añadió ash_test:"
git log --diff-filter=A --oneline -- "$ASH_TEST" 2>/dev/null | head
echo

echo "### 12. Comparación con otros testdata del repo #########################"
echo "-- todos los dirs 'testdata' por tamaño:"
find . -type d -name testdata -prune -exec du -sh {} + 2>/dev/null | sort -rh | head -20
echo

echo "### 13. Prueba de fuego: ¿rompe algo si desaparece? #####################"
echo "-- (ejecuta los tests del paquete para fijar una línea base)"
echo "   go test ./pkg/applets/ash/... -run . -count=1"
echo "-- (para verificar de verdad: mueve $ASH_TD a un tmp y repite los tests)"
echo
echo "### FIN #################################################################"
