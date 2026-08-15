# Fecha

2026-08-15

# Objetivo

Completar la experiencia multimedia y de unidades físicas: identidad estable, elevación de privilegios, desmontaje Windows sin `mountvol.exe`, progreso real de indexación, Galería offline con visor multimedia, thumbnails opcionales con FFmpeg, scroll infinito/paginación y subida contextual en Archivos.

# Decisiones tomadas

- Windows identifica el volumen por Volume GUID y conserva además el serial del filesystem. La letra (`E:`, `F:`...) es solo el punto de montaje actual.
- Linux prioriza UUID, usa PARTUUID cuando no existe UUID y conserva `/dev/disk/by-id` como identificador físico auxiliar cuando está disponible.
- Windows usa APIs Win32 para asignar/retirar mountpoints y solicita UAC al iniciar si el proceso no está elevado.
- Linux intenta `sudo` solo en ejecución interactiva sin privilegios; en systemd reconoce `CAP_SYS_ADMIN` y no relanza.
- La indexación hace una pasada de conteo y otra de procesamiento para poder reportar progreso real.
- FFmpeg es opcional: se detecta automáticamente y amplía thumbnails/previews sin convertirlo en dependencia del servidor.
- `/galeria` sustituye a `/fotos`; la ruta anterior redirige para compatibilidad.
- Todos los iconos, CSS y JavaScript son locales/embebidos. No se permite CDN para la UI base.
- Galería y Archivos usan un único concepto de listado: continuo/infinito o paginación, persistido en cookie.
- La subida desaparece de las tarjetas de Almacenamiento y pasa a un botón contextual dentro de una carpeta de Archivos.

# Arquitectura actual

- `internal/privilege`: elevación específica por SO.
- `internal/storage`: descubrimiento, identidad y mount/unmount por plataforma.
- `internal/catalog`: catálogo, indexador, progreso, thumbnails/previews y FFmpeg opcional.
- `internal/app/listing.go`: política reutilizable de listado.
- `/api/indexacion`: progreso para UI.
- `/api/galeria` y `/api/archivos/listado`: carga incremental.
- `web/components`: iconos y selector de modo reutilizables.

# Librerías usadas

- Biblioteca estándar de Go para servidor, filesystem, imágenes nativas, Win32 vía `syscall` y frontend embebido.
- FFmpeg: ejecutable opcional externo detectado por PATH; no es requisito ni módulo Go.

# Archivos importantes modificados

- `cmd/server/main.go`
- `internal/privilege/*`
- `internal/storage/platform_windows.go`
- `internal/storage/platform_linux.go`
- `internal/catalog/indexer.go`
- `internal/app/listing.go`
- `internal/app/files_handlers.go`
- `internal/app/storage_handlers.go`
- `web/components/icons.html`
- `web/components/listing_controls.html`
- `web/pages/storage.html`
- `web/pages/photos.html`
- `web/pages/files.html`
- `web/static/app.js`
- `web/static/app.css`
- `scripts/test.cmd`
- `scripts/test.sh`

# Problemas encontrados

- `mountvol /d` podía fallar con `Acceso denegado` y hacía depender el flujo de un ejecutable externo.
- La letra de unidad no es una identidad estable.
- La indexación anterior no exponía un porcentaje real por unidad.
- El formulario de upload ocupaba demasiado espacio y estaba en un lugar de administración, no en el explorador.
- La Galería solo mostraba imágenes y requería interacción manual para cargar más elementos.

# Soluciones implementadas

- Elevación automática controlada y APIs Win32 de volumen.
- Identidad persistente separada del mountpoint actual.
- Progreso `scanned/total` actualizado en vivo.
- Visor de imagen/video/audio con teclado y zoom.
- FFmpeg opcional para thumbnails multimedia y formatos de imagen adicionales.
- Scroll infinito automático y paginación seleccionable.
- Widget/dialog de subida contextual por carpeta.
- Pruebas de regresión para UI, progreso, listado e identidad visible.

# Pendientes

- SMB continúa deliberadamente pendiente hasta seleccionar una implementación servidor mantenida y con semántica de seguridad/locking suficientemente sólida.
- Probar mount/unmount contra hardware físico Windows y Linux; las pruebas automatizadas cubren lógica y compilación, no pueden simular bloqueo de volúmenes reales.

# Próximos pasos

- Probar con HDD/SSD/USB reales y confirmar que el dispositivo vuelve a montar por GUID/UUID después de cambiar letra/mountpoint.
- Probar FFmpeg con un corpus real de HEIC/WebP/AVIF/RAW/video/audio de los dispositivos del usuario.
- Evaluar SMB solo después de validar WebDAV y la capa VFS con uso real.
