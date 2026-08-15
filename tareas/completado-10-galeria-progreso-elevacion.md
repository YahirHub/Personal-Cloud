# Tarea 10 — Galería, progreso, identidad y elevación

## Objetivo

Cerrar la experiencia solicitada alrededor de unidades físicas y navegación: progreso en vivo, identidad estable, desmontaje privilegiado, subida contextual, UI offline y listado reutilizable.

## Implementado

- Volume GUID + serial en Windows.
- UUID/PARTUUID/by-id en Linux.
- Autoelevación Windows UAC y Linux sudo interactivo cuando aplica.
- Montaje/desmontaje Windows mediante APIs Win32.
- Progreso real de indexación en cada tarjeta.
- `/galeria` y redirección desde `/fotos`.
- Iconos SVG locales/offline.
- Scroll infinito o paginación persistente.
- Upload mediante botón/dialog dentro de `/archivos/ver/...`.
- Scripts únicos de prueba `scripts/test.cmd` y `scripts/test.sh`.

## Criterio para cerrar

- Tests, vet y detector de carreras limpios.
- Builds estáticos Linux/Windows.
- ZIP final probado desde una extracción limpia.
