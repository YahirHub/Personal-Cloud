# Fecha

2026-08-15

# Objetivo

Corregir el posicionamiento del visor multimedia, persistir preferencias de video, retirar de Galería el contenido de unidades desconectadas, agregar filtros/orden y proporcionar descarga segura por clic derecho sin revelar rutas ni identificadores internos en la URL.

# Decisiones tomadas

- El stage del visor ocupa todo el viewport; imagen/video usan `object-fit: contain` y controles superpuestos para no desplazar el medio hacia abajo.
- El botón de pantalla completa se muestra únicamente para video y usa Fullscreen API del navegador.
- `muted`, volumen y velocidad se persisten en `localStorage` del navegador, no en el servidor.
- Galería filtra en servidor por `storage_id` de unidades conectadas. Desmontaje idle no equivale a desconexión física.
- La UI comprueba disponibilidad cada 5 segundos: retira tarjetas desconectadas y refresca cuando reaparece una unidad. La comprobación es ligera: enumera GUID/UUID presentes sin consultar capacidad ni recorrer contenido.
- Filtros soportados: todos, imágenes, videos, audio.
- Orden soportado: fecha del archivo reciente/antigua, añadido reciente/antiguo y nombre A-Z/Z-A.
- La descarga segura usa un ticket AES-GCM opaco, aleatorio, ligado al usuario y con TTL de 2 minutos.
- El ticket no contiene ruta, nombre ni ID legibles. El endpoint final usa `Content-Disposition: attachment` y `Cache-Control: no-store`.
- Los logs HTTP sustituyen el token por `/descarga/{token}` para no persistir credenciales temporales.
- No se promete ocultar el cuerpo a un reverse proxy que termina TLS; para ese requisito se necesita TLS passthrough extremo a extremo.

# Arquitectura actual

- `internal/catalog.MediaQuery`: filtrado y orden del catálogo multimedia.
- `internal/app/downloads.go`: emisión/validación de tickets y streaming seguro.
- `/api/galeria/disponibilidad`: conjunto de unidades conectadas para sincronización de UI.
- `/api/descargas`: emisión autenticada/CSRF de ticket.
- `/descarga/{token}`: descarga autenticada con token opaco.
- `web/components/gallery_filters.html`: widget de filtros.
- `web/components/download_context.html`: menú contextual reutilizable.
- `web/static/app.js`: visor, preferencias de video, fullscreen, disponibilidad y descarga contextual.

# Librerías usadas

Solo biblioteca estándar de Go y APIs web nativas. No se agregaron módulos ni CDN.

# Archivos importantes modificados

- `internal/app/app.go`
- `internal/app/downloads.go`
- `internal/app/middleware.go`
- `internal/app/storage_handlers.go`
- `internal/app/files_handlers.go`
- `internal/app/viewmodels.go`
- `internal/catalog/catalog.go`
- `web/layouts/base.html`
- `web/components/icons.html`
- `web/components/gallery_filters.html`
- `web/components/download_context.html`
- `web/components/listing_controls.html`
- `web/pages/photos.html`
- `web/pages/files.html`
- `web/static/app.js`
- `web/static/app.css`

# Problemas encontrados

- El footer del visor participaba en el layout y reducía/desplazaba el stage útil del medio.
- Galería consultaba todo el catálogo y no distinguía si el volumen físico seguía conectado.
- El modo continuo perdía filtros si las siguientes páginas se obtenían por API.
- Los enlaces directos del original eran apropiados para visualización, pero no para una acción explícita de descarga con URL temporal.

# Soluciones implementadas

- Stage absoluto a viewport completo y controles superpuestos.
- Fullscreen para video y preferencias persistentes locales.
- Query de catálogo con storage IDs online + filtro + orden.
- Parámetros de filtro preservados en scroll infinito y paginación.
- Menú contextual en Galería y Archivos.
- Ticket AES-GCM temporal y ligado a sesión para descargas.
- Redacción de tokens de descarga en logs.
- Pruebas para query multimedia, render de controles y criptografía de ticket.

# Pendientes

- SMB continúa pendiente deliberadamente.
- Si se requiere que un proxy HTTP/TLS de terceros no pueda ver ni siquiera los bytes del archivo, desplegar TLS passthrough o diseñar un flujo de cifrado cliente-servidor específico; no mezclarlo con la descarga web normal sin evaluar compatibilidad/streaming de archivos grandes.

# Próximos pasos

- Validar el visor con videos verticales, panorámicos, 4K y fotografías de relaciones de aspecto extremas.
- Desconectar/reconectar físicamente un USB mientras Galería está abierta y comprobar ocultación/reaparición.
- Probar descargas grandes detrás del proxy real del usuario.
