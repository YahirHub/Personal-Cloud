# Tarea 11 — Visor, filtros, disponibilidad y descargas

## Objetivo

Asegurar que el visor siempre centre y aproveche el viewport, persistir preferencias de video, ocultar contenido de unidades desconectadas, permitir filtrado/orden y agregar descarga contextual segura.

## Implementado

- Stage multimedia a viewport completo con `object-fit: contain`.
- Fullscreen de video.
- Persistencia local de mute, volumen y velocidad.
- Galería limitada a unidades físicamente conectadas mediante comprobación ligera de GUID/UUID.
- Sincronización de disponibilidad en vivo.
- Filtro por imagen/video/audio.
- Orden por fecha de archivo, fecha de incorporación y nombre.
- Menú de clic derecho reutilizable en Galería y Archivos.
- Tickets AES-GCM de descarga ligados al usuario y de vida corta.
- `Content-Disposition: attachment`, no-store, HTTPS remoto obligatorio y redacción de token en logs.
- Pruebas de query y ticket cifrado.

## Criterio para cerrar

- `scripts/test.cmd` / `scripts/test.sh` limpios.
- `go test -race ./...` y `go vet ./...` limpios.
- Build estático Linux y cross-build Windows/amd64.
- ZIP final extraído y probado.
