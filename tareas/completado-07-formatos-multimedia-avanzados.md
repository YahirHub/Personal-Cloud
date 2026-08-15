# Tarea 07 — Formatos multimedia avanzados

## Objetivo

Ampliar Galería para imagen/video/audio sin introducir codecs obligatorios que rompan el binario estático del servidor.

## Implementado

- JPEG/PNG/GIF: thumbnail + preview nativos.
- Video: thumbnail de frame representativo cuando FFmpeg está disponible.
- Audio: carátula embebida cuando FFmpeg está disponible.
- WebP/HEIC/HEIF/AVIF/RAW: FFmpeg opcional genera preview/thumbnail cuando el build instalado soporta el codec.
- Visor HTML5 de imagen/video/audio.
- Navegación ←/→ y A/D.
- Zoom suave W/S.
- FFmpeg detectado mediante `exec.LookPath`, sin convertirlo en requisito.
- El servidor continúa funcionando completamente sin FFmpeg.

## Criterio para cerrar

- Tests y vet limpios.
- Build Windows/Linux sin CGO.
- Galería sin dependencias CDN.
- La ausencia de FFmpeg no debe impedir indexar ni arrancar.
