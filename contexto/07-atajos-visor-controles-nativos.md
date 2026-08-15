# Fecha
2026-08-15

# Objetivo
Corregir la pérdida de atajos del visor multimedia después de que el usuario interactúa con seek, volumen u otros controles nativos del reproductor.

# Decisiones tomadas
Los controles multimedia del navegador pueden consumir eventos de teclado dentro de su UI nativa. No se fuerza el foco fuera del reproductor porque eso deterioraría accesibilidad y uso normal. El visor escucha `keydown` en `window` durante la fase de captura para interceptar únicamente sus teclas reservadas antes de que lleguen al control nativo.

# Arquitectura actual
La navegación del visor continúa centralizada en `web/static/app.js`. No se agregan dependencias ni listeners duplicados por cada elemento multimedia.

# Librerías usadas
Ninguna nueva. JavaScript nativo del navegador.

# Archivos importantes modificados
- `web/static/app.js`
- `web/assets_test.go`
- `tareas/completado-13-atajos-visor-controles-nativos.md`

# Problemas encontrados
Tras pulsar controles nativos de `<video>`, el foco quedaba dentro del reproductor y el evento `keydown` podía ser consumido antes de llegar al listener de `document` usado por el visor.

# Soluciones implementadas
- Listener `keydown` en `window` con captura habilitada.
- `preventDefault` y `stopPropagation` solo para A/D, flechas, W/S y Escape mientras el visor está abierto.
- Se respetan combinaciones con Ctrl/Meta/Alt y campos editables.

# Pendientes
- SMB continúa pendiente como tarea separada.

# Próximos pasos
Validar manualmente en Chrome/Edge/Firefox que después de mover el seek o volumen siguen funcionando A/D, flechas y W/S.
