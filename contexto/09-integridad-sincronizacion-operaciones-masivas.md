# Fecha
2026-08-15

# Objetivo
Agregar detección conservadora de medios dañados, reconciliación manual/periódica del catálogo y operaciones seguras sobre uno o varios archivos: mover, eliminar y descargar como ZIP streaming, incluyendo acciones táctiles y selección múltiple.

# Decisiones tomadas
- La integridad se clasifica como `ok`, `damaged` o `unchecked`; nunca se marca un archivo como dañado si el servidor no puede verificarlo con suficiente certeza.
- Imágenes compatibles con la stdlib se decodifican completamente hasta el límite de seguridad existente; video/audio y formatos no nativos usan `ffprobe` cuando está disponible.
- La validación es operativa y conservadora, no un análisis forense de todos los frames/bits. No se obliga a decodificar videos completos durante cada sincronización para no castigar CPU/discos.
- Omitir dañados conserva los archivos y silencia el aviso mientras no cambien. Eliminar dañados retira original, caché y entrada de catálogo.
- La sincronización periódica global queda desactivada por defecto, con intervalo configurable de 5 minutos a 7 días. Solo se encolan unidades conectadas.
- El movimiento dentro de una unidad usa rename. Entre unidades usa copia streaming a temporal + Sync + confirmación + borrado del origen. Luego se actualiza el catálogo directamente, sin reescaneo total.
- Las descargas múltiples se generan como ZIP directamente sobre HTTP, con un archivo abierto a la vez y buffer de 64 KiB. Los formatos ya comprimidos usan `zip.Store`; los compresibles usan Deflate `BestSpeed` para equilibrar tamaño y CPU. Solo se permite un ZIP masivo simultáneo.
- Los tickets de ZIP son aleatorios, opacos, de un solo uso, ligados al usuario y caducan en 5 minutos. Fuera de loopback se exige HTTPS.
- Las operaciones masivas tienen máximo 500 elementos y rate limit por usuario/IP.
- Clic derecho y pulsación larga táctil comparten el mismo menú de acciones.

# Arquitectura actual
- `internal/catalog`: catálogo persistente, salud de archivos, reconciliación y estado de indexación.
- `internal/vfs`: movimiento seguro intra/inter-unidad y operaciones de archivos bajo leases.
- `internal/app/settings.go`: configuración de sincronización manual/periódica.
- `internal/app/damaged_handlers.go`: omitir/eliminar medios dañados.
- `internal/app/operations.go`: selección masiva, mover, eliminar, ZIP streaming y reconciliación puntual de archivos faltantes.
- `web/components/bulk_actions.html`: barra y diálogo de movimiento reutilizables.
- `web/static/app.js`: selección, menú contextual, pulsación larga y operaciones AJAX protegidas por CSRF.

# Librerías usadas
No se agregaron dependencias Go externas. Se usan `archive/zip`, `image`, `io`, `crypto/rand` y demás biblioteca estándar existente. `ffprobe`/FFmpeg siguen siendo ejecutables externos opcionales detectados en el sistema.

# Archivos importantes modificados
- `internal/catalog/catalog.go`
- `internal/catalog/indexer.go`
- `internal/catalog/integrity.go`
- `internal/store/store.go`
- `internal/store/migrations.go`
- `internal/vfs/fs.go`
- `internal/app/app.go`
- `internal/app/settings.go`
- `internal/app/damaged_handlers.go`
- `internal/app/operations.go`
- `internal/app/downloads.go`
- `web/pages/settings.html`
- `web/pages/storage.html`
- `web/pages/photos.html`
- `web/pages/files.html`
- `web/components/bulk_actions.html`
- `web/components/download_context.html`
- `web/static/app.js`
- `web/static/app.css`

# Problemas encontrados
- Un archivo eliminado directamente desde Explorer/terminal podía seguir apareciendo hasta la siguiente indexación.
- Mover entre unidades no puede resolverse con rename y debe tolerar cortes/errores sin borrar el origen antes de confirmar el destino.
- Crear ZIPs temporales grandes o comprimir video/fotos con deflate puede consumir demasiado almacenamiento, RAM o CPU en un MiniPC modesto.
- Determinar corrupción completa de video/audio exigiría decodificar streams completos y sería demasiado costoso para una sincronización frecuente.

# Soluciones implementadas
- Si una apertura/descarga encuentra `os.ErrNotExist`, se retira inmediatamente la entrada faltante del catálogo y sus caches.
- La sincronización vuelve a caminar la unidad y reconcilia nuevos, modificados y eliminados externos.
- Movimiento cross-volume transaccional a nivel de archivo: destino temporal sincronizado antes de retirar origen.
- Actualización directa de ID/storage/ruta del catálogo y renombrado de caches tras mover.
- ZIP streaming de bajo consumo, sin archivo temporal global, con compresión adaptativa (`Store` para media/archivos ya comprimidos y Deflate `BestSpeed` para texto) y reporte interno de elementos que fallaron durante el stream.
- Detección conservadora de medios dañados y aviso por unidad al finalizar la indexación.
- Selección múltiple y acciones equivalentes por ratón o touch, totalmente offline.

# Pendientes
- SMB permanece pendiente hasta elegir una implementación servidor Go suficientemente madura/licenciable.
- Una validación forense opcional que decodifique streams completos podría añadirse en el futuro como tarea manual explícita, nunca dentro de la sincronización normal.

# Próximos pasos
- Probar con unidades reales Windows/Linux: archivo truncado, archivo borrado externamente, archivo nuevo externo, movimiento entre dos USB y desconexión durante un ZIP.
- Validar sincronización periódica con HDD que usan auto-unmount y ajustar intervalo según el uso real.
