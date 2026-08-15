# Fecha
2026-08-15

# Objetivo
Cerrar los huecos detectados entre backend y experiencia visible de integridad, sincronización y operaciones de archivos.

# Decisiones tomadas
- Mantener un solo worker de indexación para hardware modesto.
- Separar sincronización normal de verificación forzada de integridad.
- Video/audio usan metadata + muestras de decodificación FFmpeg cuando está disponible.
- `unchecked` nunca se presenta como dañado.
- Mover permite navegar/crear carpeta destino; el movimiento sigue actualizando el catálogo directamente.
- Descargas masivas siguen por streaming, con un solo ZIP simultáneo y sin temporales gigantes.

# Arquitectura actual
- `catalog.Indexer.Enqueue`: reconciliación normal.
- `catalog.Indexer.EnqueueVerify`: fuerza revalidación de multimedia no modificada.
- `/configuracion`: centro de sincronización e integridad por unidad.
- `/api/carpetas` y `/api/carpetas/crear`: selector seguro de destino para movimientos.

# Librerías usadas
Solo biblioteca estándar Go. FFmpeg/ffprobe continúan siendo ejecutables externos opcionales.

# Archivos importantes modificados
- internal/catalog/indexer.go
- internal/catalog/integrity.go
- internal/catalog/catalog.go
- internal/app/settings.go
- internal/app/operations.go
- web/pages/settings.html
- web/components/bulk_actions.html
- web/components/gallery_filters.html
- web/static/app.js
- web/static/app.css

# Problemas encontrados
R12 tenía varias capacidades implementadas pero poco visibles y la verificación de video/audio se limitaba a metadata.

# Soluciones implementadas
Centro de integridad visible, diferencias de sincronización, verificación forzada, muestreo FFmpeg, selector de carpetas con creación explícita e iconos SVG locales en filtros.

# Pendientes
SMB sigue pendiente hasta elegir una implementación servidor suficientemente madura y compatible con la política del proyecto.

# Próximos pasos
Validar con unidades físicas del usuario, especialmente medios corruptos reales y movimientos entre filesystems distintos.
