# Tarea 04 — Catálogo, miniaturas y previews

## Objetivo

Navegar el catálogo de fotos desde el almacenamiento interno sin mantener montada la unidad que contiene el original.

## Alcance completado

- Catálogo persistente separado del estado de autenticación.
- Snapshot compacto + journal JSONL append-only.
- IDs estables por `storage_id + ruta relativa`.
- Indexador de una sola cola para evitar golpear varios HDD simultáneamente.
- Reconciliación de archivos borrados y modificados.
- Miniatura JPEG de hasta 320 px.
- Preview JPEG de hasta 1600 px.
- Caché privada dentro de `data/cache/`.
- Galería `/galeria` con lazy loading y paginación.
- Abrir miniatura/preview no monta el disco original.
- Abrir el original resuelve el VFS, monta su unidad y mantiene el lease hasta terminar el stream.
- Al modificar una imagen en la misma ruta se invalida y regenera su caché.
- Backups diarios de metadatos usan un snapshot consistente del catálogo.

## Límite deliberado

La biblioteca estándar decodifica para thumbnail JPEG, PNG y GIF. Por protección de memoria, fuentes de más de 80 megapíxeles quedan catalogadas pero sin thumbnail en esta implementación. Otros formatos reconocidos (HEIC/HEIF/AVIF/RAW/WebP) se catalogan como imágenes pero actualmente pueden quedar sin miniatura. Video todavía no genera frame de portada. Esta ampliación se registra como tarea 07 en vez de introducir dependencias pesadas dentro de esta entrega.
