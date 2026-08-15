# Fecha
2026-08-15

# Objetivo
Corregir el menú de tres puntos para exponer acciones explícitas de selección y mover calidad/pantalla completa del video a una barra inferior integrada con los controles de reproducción.

# Decisiones tomadas
- `⋯` ya no activa selección directamente: abre un menú reutilizable con `Seleccionar` y `Seleccionar todo`.
- `Seleccionar todo` completa el listado cuando se usa scroll infinito y selecciona hasta 500 elementos disponibles, respetando el límite seguro de operaciones masivas.
- Se reemplazaron los controles nativos del `<video>` por controles HTML/CSS/JS locales propios para conservar funcionamiento 100% offline.
- Calidad FFmpeg y fullscreen viven en la misma barra inferior que play, progreso, volumen, tiempo y velocidad.
- Fullscreen se solicita sobre el shell completo del visor para conservar visibles los controles personalizados.
- Volumen, mute y velocidad continúan persistiendo en `localStorage`.
- Se evaluó un reproductor externo ligero, pero no se agregó una dependencia runtime/CDN: el producto sigue sin necesitar Internet para cargar la UI.

# Arquitectura actual
El `<video>` permanece como elemento HTML5 de reproducción/streaming, pero `controls=false`. La UI local controla el elemento mediante su API estándar. Las variantes FFmpeg siguen siendo servidas por Go con soporte de rangos y el selector de calidad solo cambia la fuente conservando tiempo y preferencias.

# Librerías usadas
Ninguna nueva.

# Archivos importantes modificados
- `web/pages/photos.html`
- `web/pages/files.html`
- `web/components/selection_menu.html`
- `web/components/icons.html`
- `web/layouts/base.html`
- `web/static/app.js`
- `web/static/app.css`
- `web/assets_test.go`
- `internal/app/app_test.go`
- `README.md`

# Problemas encontrados
- El botón de tres puntos activaba directamente el modo selección y no ofrecía las dos opciones pedidas.
- Calidad y fullscreen estaban flotando separados del reproductor.
- Usar controles nativos limita la capacidad de colocar controles propios exactamente en la barra inferior de forma uniforme entre navegadores.

# Soluciones implementadas
- Menú anclado al botón `⋯` con selección simple/total.
- Barra de controles de video propia en la parte inferior.
- Fullscreen del visor completo con fallback móvil.
- Seek, play/pause, volumen, mute, velocidad y tiempo gestionados localmente.
- Pruebas de regresión para menú, selector de calidad y controles inferiores.

# Pendientes
- SMB permanece como tarea deliberadamente pendiente.

# Próximos pasos
Probar visualmente R14 en Chrome/Edge de Windows y Chrome Android con videos originales y variantes FFmpeg, incluyendo selección total sobre listados largos.
