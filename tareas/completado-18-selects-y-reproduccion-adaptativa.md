# Tarea 18 — Selects persistentes y reproducción adaptativa

## Objetivo

- Evitar dropdowns nativos blancos/inconsistentes en modo oscuro.
- Aplicar el estilo de selects de forma global.
- Agregar calidad automática de video según ancho de banda y tamaño del visor.
- Pausar, mostrar loader y reanudar al aplicar un cambio de resolución.
- Hacer fluida la línea de tiempo del reproductor con actualización por frame.

## Estado

completado

## Validación requerida

- `scripts/test.sh` / `scripts\\test.cmd`.
- `go test -race ./...`.
- `node --check web/static/app.js`.
- build estático Linux y Windows/amd64.
