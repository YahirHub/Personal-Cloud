# Fecha
2026-08-15

# Objetivo
Eliminar el parpadeo de orientación al navegar entre fotografías e incorporar reproducción de video en resoluciones alternativas cuando FFmpeg está instalado, manteniendo el servidor como binario Go estático y funcional sin FFmpeg.

# Decisiones tomadas
- La orientación de JPEG se normaliza al generar thumbnail/preview leyendo `EXIF Orientation` y aplicando las ocho transformaciones definidas por EXIF.
- La caché de imagen se versiona (`CacheVersion=2`) para forzar regeneración al reindexar archivos cuya preview antigua no había aplicado orientación.
- El visor precarga y decodifica una imagen fuera del DOM antes de sustituir la visible. Para cachés antiguas evita mostrar la preview potencialmente incorrecta y espera al original decodificado.
- FFmpeg es opcional y se ejecuta como proceso externo. No se enlazan libav ni codecs por CGO.
- La reproducción multiresolución requiere FFmpeg con `libx264`; sin él se conserva reproducción del original.
- Las variantes se crean bajo demanda como MP4 (`faststart`) en el SSD interno y Go las sirve con `http.ServeContent` para conservar Range/seek del reproductor HTML5.
- Se limita a una transcodificación simultánea para evitar saturar el MiniPC.
- Variantes disponibles: 360p, 480p, 720p y 1080p, nunca superiores a la resolución fuente cuando esta se conoce.
- `ffprobe`, cuando está disponible, obtiene dimensiones y rotación de videos antiguos que todavía no tenían esa metadata en el catálogo.
- Las variantes no son respaldo: son caché regenerable y se purgan al superar 72 horas de antigüedad.

# Arquitectura actual
```text
Galería
  -> imagen: preview cache v2 (orientada) -> decode() -> swap -> original
  -> video: original HTML5
       -> selector calidad
       -> POST preparar variante
       -> Streaming Manager (1 worker)
       -> VFS lease / montaje de unidad
       -> ffmpeg externo
       -> data/cache/video-variants/<file>/<fingerprint>/<calidad>.mp4
       -> http.ServeContent / Range
       -> reproductor conserva tiempo/volumen/mute/velocidad
```

# Librerías usadas
- Biblioteca estándar Go para HTTP, caché, EXIF JPEG, imágenes y procesos.
- Cero módulos Go externos.
- FFmpeg/ffprobe: ejecutables externos opcionales detectados con `exec.LookPath`.

# Archivos importantes modificados
- `internal/catalog/catalog.go`
- `internal/catalog/indexer.go`
- `internal/catalog/orientation.go`
- `internal/catalog/orientation_test.go`
- `internal/streaming/manager.go`
- `internal/streaming/manager_test.go`
- `internal/app/app.go`
- `internal/app/video_handlers.go`
- `web/pages/photos.html`
- `web/static/app.js`
- `web/static/app.css`
- `web/assets_test.go`
- `README.md`

# Problemas encontrados
- Las previews JPEG anteriores se re-encodeaban sin aplicar `EXIF Orientation`, mientras el navegador sí orientaba el original; al cambiar de fotografía se veía brevemente la preview acostada y luego el original correcto.
- Un pipe directo de FFmpeg al response impediría una experiencia de seek/rangos tan robusta como la reproducción de un archivo estable y mantendría la unidad/CPU ocupada en cada reproducción.
- Algunos videos históricos no tenían ancho/alto catalogado, por lo que no era posible filtrar correctamente las resoluciones ofrecidas.

# Soluciones implementadas
- Parser EXIF mínimo y transformador `image.Image` sin dependencia externa.
- Caché v2 + regeneración en reindexación.
- Predecodificación de imágenes antes de intercambiar el elemento visible.
- Gestor de variantes MP4 bajo demanda con fingerprint por tamaño/mtime, caché, cola única y retención.
- Endpoints autenticados para listar, preparar, consultar y servir calidades.
- Detección opcional de dimensiones/rotación con ffprobe y persistencia en catálogo.
- Selector de calidad integrado al visor, conservando estado de reproducción al cambiar de fuente.

# Pendientes
- Evaluar aceleración por hardware como opción avanzada solo si existe una necesidad real y se puede hacer sin degradar portabilidad.
- SMB permanece pendiente por decisión arquitectónica previa.

# Próximos pasos
- Probar en el MiniPC con videos reales H.264/H.265 y varias resoluciones.
- Reindexar una unidad con fotos EXIF rotadas para regenerar la caché v2.
- Medir CPU/tiempo de FFmpeg y ajustar preset/concurrencia únicamente con datos reales.
