# Tarea 12 — Corregir visor y estado offline

## Objetivo

Corregir el solapamiento de metadatos del visor con los controles nativos de video, marcar claramente en Archivos las unidades desconectadas y auditar que toda la interfaz funcione sin recursos externos ni CDN.

## Alcance

- Reubicar metadatos/ayuda del visor de video fuera de la zona de controles nativos.
- Mantener imágenes y videos centrados y ajustados al viewport.
- Marcar raíces y filas de unidades desconectadas con estado visual gris sin eliminar el catálogo navegable.
- Mantener el estado offline también en filas cargadas por scroll infinito.
- Auditar HTML/CSS/JS embebido para impedir dependencias remotas.
- Agregar pruebas de regresión para visor, estado offline y recursos locales.

## Implementado

- Metadatos del visor movidos a overlay superior.
- Estado gris para unidades y filas desconectadas en Archivos.
- Estado offline aplicado también en listado continuo.
- Auditoría automática de recursos remotos en todos los assets embebidos.
- Pruebas de regresión de visor y estado offline.

## Criterio para cerrar

- `scripts/test.cmd` / `scripts/test.sh` limpios.
- `go test -race ./...` y `go vet ./...` limpios.
- JS válido.
- Build estático Linux y Windows/amd64.
- Cero referencias de recursos UI a CDN/HTTP externos.
