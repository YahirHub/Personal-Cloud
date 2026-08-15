# Tarea 19 — Cambio de calidad sin interrupción

## Objetivo

- Mantener el video actual reproduciéndose mientras Auto o una selección manual preparan otra resolución.
- Cambiar automáticamente a la variante solicitada cuando esté lista.
- No mostrar loader ni pausar visualmente por un cambio de calidad.
- Usar loader únicamente al abrir un video mientras aún no hay datos reproducibles.
- Mantener posición, mute, volumen, velocidad y timeline al hacer el swap.

## Estado

completado

## Validación requerida

- `scripts/test.sh` / `scripts\\test.cmd`.
- `go test -race ./...`.
- `node --check web/static/app.js`.
- build estático Linux y Windows/amd64.
