# Fecha
2026-08-15

# Objetivo
Cerrar la inconsistencia de R5 donde el catálogo indicaba que debía pulsarse Indexar pero una unidad recién detectada solo mostraba Registrar, y completar el explorador web/routing automático.

# Decisiones tomadas
- Registrar una unidad es una acción explícita porque primero deben definirse categoría, raíz virtual, solo lectura y timeout.
- Una vez registrada, la primera indexación se encola automáticamente.
- Las indexaciones posteriores se ejecutan con `Indexar ahora`.
- `/archivos` usa el catálogo para listar y no monta unidades solo para navegar.
- La subida automática prioriza una unidad especializada y usa una unidad mixta como fallback.
- Entre destinos de igual prioridad se utiliza el mayor espacio libre conocido.
- La descarga del original conserva el VFS/lease existente y monta solo la unidad necesaria.

# Arquitectura actual
Web -> catálogo para listados offline -> VFS para originales/escrituras -> Storage Manager -> unidad física.

La raíz `/archivos` muestra las raíces virtuales registradas. Las subcarpetas se derivan de `catalog.File.RelativePath`.

# Librerías usadas
Solo biblioteca estándar de Go. No se agregaron módulos externos.

# Archivos importantes modificados
- `internal/app/files_handlers.go`
- `internal/app/storage_handlers.go`
- `internal/app/app.go`
- `internal/app/viewmodels.go`
- `internal/catalog/catalog.go`
- `web/pages/files.html`
- `web/pages/storage.html`
- `web/pages/photos.html`
- `web/components/sidebar.html`
- `web/layouts/base.html`
- `web/static/app.js`
- `web/static/app.css`
- `README.md`

# Problemas encontrados
R5 tenía backend e indexador, pero el botón Indexar se renderizaba únicamente para `Registered=true`. La tarjeta de volumen detectado exigía registrar primero y no iniciaba automáticamente la indexación, haciendo que el mensaje de Fotos pareciera referirse a una acción inexistente.

# Soluciones implementadas
- Botón `Registrar e indexar`.
- Encolado automático después de un registro correcto.
- `Indexar ahora` persistente para unidades registradas.
- Auto-refresh ligero mientras el job está activo.
- Explorador y subida automática.
- Tests del routing automático, traversal y navegación derivada del catálogo.

# Pendientes
- Tarea 07: formatos multimedia avanzados.
- Tarea 09: SMB; seguir evaluándolo antes de añadir una implementación de servidor inmadura.

# Próximos pasos
Validar en hardware real el primer registro/indexación y revisar formatos multimedia que el usuario use realmente antes de introducir decoders adicionales.
