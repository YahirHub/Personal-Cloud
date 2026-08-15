# Fecha

2026-08-15

# Objetivo

Corregir el solapamiento visual entre metadatos del visor y controles HTML5 de video, hacer explícito en Archivos cuándo una unidad física está desconectada y verificar que toda la interfaz web sea autosuficiente/offline.

# Decisiones tomadas

- Los metadatos del visor se mueven a una capa superior absoluta (`viewer-meta`) y nunca reservan espacio del stage ni ocupan la zona inferior de controles nativos.
- Imagen y video continúan ocupando `100% x 100%` del stage con `object-fit: contain` y centro geométrico.
- Una unidad desconectada no desaparece de Archivos porque su catálogo sigue siendo útil; se muestra gris/atenuada y marcada como `No disponible · catálogo local`.
- Las filas de carpetas/archivos de un volumen desconectado reutilizan la misma clase visual `is-offline`, incluyendo elementos cargados dinámicamente por scroll infinito.
- No se agrega Service Worker ni caché web artificial: el servidor local ya sirve HTML/CSS/JS/SVG desde el binario. Para funcionamiento sin Internet basta con no depender de recursos remotos.
- Se agrega una prueba que recorre todos los assets web embebidos y rechaza referencias HTTP/HTTPS/protocol-relative.

# Arquitectura actual

- `web/pages/photos.html`: visor con stage independiente y capa `viewer-meta`.
- `web/pages/files.html`: estados visuales offline para raíces y elementos catalogados.
- `web/static/app.css`: estilos del visor y de disponibilidad.
- `web/static/app.js`: replica el estado offline en filas creadas por listado continuo.
- `web/assets_test.go`: auditoría automática de recursos remotos y regresión de posición del visor.

# Librerías usadas

No se agregaron librerías, módulos ni CDN. Toda la UI continúa embebida mediante `embed.FS`.

# Archivos importantes modificados

- `web/pages/photos.html`
- `web/pages/files.html`
- `web/static/app.css`
- `web/static/app.js`
- `web/assets_test.go`
- `internal/app/app_test.go`
- `README.md`

# Problemas encontrados

- La franja inferior de ayuda del visor quedaba inmediatamente encima de los controles nativos de un video y podía solaparse visualmente con ellos en viewports bajos.
- Archivos comunicaba textualmente la desconexión, pero la raíz de la unidad seguía teniendo el mismo aspecto que una unidad disponible.
- La afirmación de funcionamiento offline existía en documentación, pero no había una prueba global que evitara introducir accidentalmente un CDN en el futuro.

# Soluciones implementadas

- Metadatos movidos a la parte superior del visor mediante overlay absoluto.
- Estado `is-offline` reutilizable para tarjetas de unidad y filas del explorador.
- Indicador `No disponible · catálogo local` en raíces desconectadas.
- Estado offline conservado en filas generadas por JavaScript.
- Test global de assets embebidos sin referencias remotas.
- Test de renderizado para raíces/filas offline y regresión del visor.

# Pendientes

- SMB continúa pendiente como tarea 09.

# Próximos pasos

- Validar visualmente con video horizontal, vertical y ultrawide en el navegador del usuario.
- Desconectar/reconectar una unidad mientras `/archivos` está abierto y comprobar el estilo tras recargar/navegar.
