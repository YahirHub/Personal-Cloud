# Fecha
2026-08-15

# Objetivo
Corregir dropdowns blancos/inconsistentes en modo oscuro y hacer el reproductor de video adaptativo y fluido.

# Decisiones tomadas
- Los `select`, `option` y `optgroup` reciben tema explícito global para evitar que Windows/Chromium pinte un popup blanco sobre UI oscura.
- Los selects dentro del reproductor fuerzan `color-scheme: dark` porque el visor siempre usa fondo oscuro.
- La calidad predeterminada de video con FFmpeg es `Auto`.
- Auto mide hasta 512 KiB de la URL original con `Range`, por la misma ruta/proxy del usuario, y combina la medición con `navigator.connection` cuando existe y el tamaño efectivo del visor.
- Se usa margen de seguridad de ancho de banda y un factor de penalización cuando aparecen eventos `waiting`/`stalled`.
- Las variantes FFmpeg conservan CRF pero añaden `maxrate/bufsize` por perfil: 360p 900k, 480p 1600k, 720p 3200k y 1080p 5800k; esto hace predecible la heurística de Auto.
- La caché de variantes sube a versión `v2` para no reutilizar MP4 antiguos generados sin esos límites.
- Una variante automática se prepara en segundo plano y solo pausa al momento del swap; un cambio manual pausa inmediatamente.
- El swap conserva tiempo, estado de reproducción, mute, volumen y velocidad y muestra loader local.
- La timeline se actualiza mediante `requestAnimationFrame`, no depende de `timeupdate`.

# Arquitectura actual
El backend continúa generando variantes MP4 mediante FFmpeg externo opcional. El algoritmo adaptativo vive en el frontend embebido y no añade dependencias de runtime.

# Librerías usadas
Ninguna nueva. Biblioteca estándar Go + JavaScript/CSS local.

# Archivos importantes modificados
- `web/static/app.css`
- `web/static/app.js`
- `web/pages/photos.html`
- `web/assets_test.go`
- `internal/streaming/manager.go`
- `internal/streaming/manager_test.go`
- `README.md`

# Problemas encontrados
El popup nativo del select no heredaba de forma fiable el tema oscuro en Windows. La línea de tiempo dependía de eventos multimedia de frecuencia variable y un control de rango con precisión limitada.

# Soluciones implementadas
Tema explícito para controles de selección, calidad Auto con medición real y fallback, loader durante swap y actualización por frame de la timeline.

# Pendientes
SMB continúa como tarea separada.

# Próximos pasos
Validar la heurística Auto sobre conexiones LAN y vía proxy real del usuario y ajustar umbrales solo si las mediciones reales lo justifican.
