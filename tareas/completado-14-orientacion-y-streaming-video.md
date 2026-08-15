# Fecha
2026-08-15

# Estado
Completado y verificado.

# Objetivo
Eliminar el parpadeo de orientación al cambiar imágenes e incorporar variantes de video por resolución usando FFmpeg externo opcional sin romper el binario Go estático.

# Implementado
- Parser mínimo de EXIF Orientation para JPEG y aplicación de las ocho orientaciones al generar caché.
- Caché de imagen versionada (`CacheVersion=2`) para regenerar previews anteriores durante reindexación.
- Predecodificación del medio nuevo antes de sustituir el visible; las previews antiguas no orientadas se omiten para evitar el destello acostado.
- Detección de `ffmpeg`, `ffprobe` y `libx264` mediante `exec.LookPath`/consulta de encoders.
- Perfiles Original, 360p, 480p, 720p y 1080p sin upscaling cuando se conoce la resolución fuente.
- Variantes MP4 bajo demanda con `faststart`, H.264/AAC y caché interna separada de originales.
- Una sola transcodificación simultánea para limitar carga del MiniPC.
- Selector de calidad integrado al visor y cambio de fuente conservando tiempo de reproducción, pausa, mute, volumen y velocidad.
- Estado queued/transcoding/ready/error consultable por API.
- `ffprobe` completa dimensiones/rotación de videos cuando falta metadata y la persiste en catálogo.
- Limpieza periódica de variantes con más de 72 h de antigüedad.
- Funcionamiento original intacto cuando FFmpeg/libx264 no están disponibles.

# Pruebas
- Tests unitarios de EXIF orientation.
- Tests de perfiles, fingerprint y rechazo de calidad arbitraria.
- Pruebas de assets para predecodificación y selector de calidad.
- `scripts/test.sh`.
- `go test -race ./...`.
- `node --check web/static/app.js`.
- Build Linux amd64 `CGO_ENABLED=0`, verificado como estático.
- Cross-build Windows amd64 `CGO_ENABLED=0`.
- Smoke FFmpeg real: fuente 640x480 -> variante 480x360 con los mismos parámetros de transcodificación.
- `git diff --check` y `git fsck --full`.
