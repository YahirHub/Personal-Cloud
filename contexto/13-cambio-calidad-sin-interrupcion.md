# Fecha
2026-08-15

# Objetivo
Hacer que los cambios de calidad automáticos y manuales no interrumpan visualmente la reproducción ni muestren loader; reservar el loader al arranque inicial del video.

# Decisiones tomadas
- Auto continúa eligiendo calidad según ancho de banda y tamaño del visor.
- La solicitud de una calidad manual tampoco pausa el video actual mientras FFmpeg prepara la variante.
- Cada solicitud lleva una secuencia local; una selección posterior invalida la espera anterior en el navegador sin bloquear el reproductor.
- La nueva fuente se precarga en un segundo `<video>` oculto, se sincroniza al `currentTime` del activo y, si estaba reproduciendo, arranca silenciada antes del intercambio.
- El video visible anterior solo se pausa cuando la fuente nueva ya está reproduciéndose/preparada, por lo que no existe una pausa visible durante el cambio.
- El loader del visor se limita a la carga inicial, hasta que el primer video alcanza estado reproducible (`canplay`/`playing`).
- Se conservan posición, mute, volumen, velocidad, controles personalizados, timeline fluida y fullscreen.

# Arquitectura actual
El backend de variantes FFmpeg no cambia. La transición transparente vive en JavaScript embebido: `prepareVideoQuality` solicita/prepara y `stageVideoSourceSwap` realiza precarga, sincronización y swap.

# Librerías usadas
Ninguna nueva. JavaScript nativo y backend Go existente.

# Archivos importantes modificados
- `web/static/app.js`
- `web/static/app.css`
- `web/assets_test.go`
- `README.md`
- `tareas/completado-18-selects-y-reproduccion-adaptativa.md`
- `contexto/12-selects-y-reproduccion-adaptativa.md`

# Problemas encontrados
El flujo R15 pausaba manualmente el video y mostraba un overlay durante el cambio de calidad. Auto también detenía brevemente el medio al aplicar la variante preparada.

# Soluciones implementadas
Precarga de la variante en un segundo elemento de video invisible, sincronización temporal y swap cuando está listo, manteniendo la fuente actual activa mientras se prepara.

# Pendientes
SMB continúa separado. La transición se debe probar además en navegadores móviles reales porque las políticas de autoplay pueden variar; la fuente de staging se inicia silenciada para maximizar compatibilidad.

# Próximos pasos
Validar el swap transparente en Chrome/Edge Android y escritorio a través del proxy real y ajustar únicamente si un navegador concreto presenta una política de reproducción distinta.
