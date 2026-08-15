# Tarea 07 — Formatos multimedia avanzados

Añadir thumbnails/previews para HEIC/HEIF/AVIF/WebP/RAW y portadas de video.

## Investigación 2026-08-15

- WebP tiene decoder oficial en `golang.org/x/image/webp`, pero introducirlo rompería temporalmente la propiedad actual de cero módulos externos. Agregarlo cuando se decida ampliar formatos, no como dependencia accidental de esta corrección.
- Existen decoders HEIF/HEIC en Go, pero las alternativas puramente Go disponibles son todavía recientes y HEVC además tiene consideraciones de patentes/licenciamiento para distribución comercial.
- Las opciones AVIF mantenidas actualmente suelen depender de CGO o de runtimes/WASM con codecs compilados, por lo que no cumplen tan limpiamente el objetivo de binario Go estático y simple.
- Portadas de video requieren demux/codec suficiente; no se añadirá FFmpeg como requisito base sin una decisión explícita.

## Criterio para cerrar

Elegir decoders mantenidos que funcionen Windows/Linux, conservar límites de píxeles/memoria, mantener compilación reproducible y añadir corpus de pruebas por formato.
