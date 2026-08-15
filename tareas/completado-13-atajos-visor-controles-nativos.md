# Fecha
2026-08-15

# Objetivo
Mantener los atajos de navegación y zoom del visor después de interactuar con los controles nativos de video o audio.

# Estado
Completado y verificado.

# Implementación
- El manejador de teclado del visor se registra en `window` en fase de captura.
- Se conservan A/D y flechas izquierda/derecha para cambiar de medio.
- Se conservan W/S para zoom.
- Escape continúa cerrando el visor.
- Ctrl, Alt y Meta no se interceptan.
- Inputs, textareas y selects siguen excluidos para no romper escritura.
- Play/pause, seek, volumen y demás controles nativos con mouse continúan intactos.

# Pruebas
- Prueba automática que exige el listener `keydown` en fase de captura.
- Suite general de `scripts/test.sh`.
- `go test -race ./...`.
