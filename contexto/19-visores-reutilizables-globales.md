# 19. Visores reutilizables globales

Fecha: 2026-08-15

## Objetivo

Eliminar la dependencia accidental entre el reproductor multimedia y la página `/galeria`. Todo archivo que Personal Cloud sabe visualizar debe abrir el visor correspondiente desde cualquier superficie que lo muestre: Página principal, Mi unidad, búsqueda, Recientes, Destacados, Galería y el menú `⋯ → Abrir`.

## Visor multimedia global

El diálogo multimedia dejó de vivir dentro de `web/pages/photos.html` y pasó a `web/components/media_viewer.html`. El layout autenticado lo monta una sola vez, igual que el visor de documentos, por lo que sus controles de imagen, video y audio existen en todas las páginas.

`web/static/app.js` expone `PersonalCloudMediaViewer.open(fileID)`. Al abrir un archivo fuera de Galería consulta `/api/archivo/{id}/info`, valida que la unidad siga disponible y reutiliza exactamente el mismo stage/reproductor que Galería.

Se conservan:

- zoom y orientación de imágenes;
- previews locales para formatos que el navegador no decodifica de forma nativa;
- reproductor de video propio;
- modo Auto y cambio transparente de calidad;
- volumen/velocidad persistentes;
- pantalla completa;
- audio con carátula/miniatura cuando existe;
- Destacados desde el visor;
- menú contextual y descarga segura;
- atajos de teclado.

## Navegación entre medios

Dentro de Galería, anterior/siguiente conserva el listado y el scroll infinito existentes. Fuera de Galería, el visor crea una secuencia con los archivos multimedia visualizables presentes en la página actual. Así se puede pasar entre imágenes/videos/audios de Mi unidad, búsqueda, Recientes o Destacados sin abandonar el visor. Los botones se ocultan cuando sólo existe un medio disponible.

## Contrato del backend

`fileViewerKind` ahora también anuncia `image`, `video` y `audio`, además de `markdown`, `html`, `text` y `pdf`. Los modelos de Inicio/Archivos y el API de listado heredan esa clasificación automáticamente.

`GET /api/archivo/{id}/info` incluye también las URLs locales necesarias para reproducir el medio (`original_url`, `thumbnail_url`, `preview_url`) y la versión de caché. De ese modo el frontend no reconstruye rutas ni duplica lógica específica de Galería.

## Texto y código compatibles

El visor de texto se amplió a formatos que pueden visualizarse/editarse de forma segura como UTF-8: CSV/TSV, JSON/JSONL, YAML, TOML, XML, INI/CFG/CONF, `.env`, propiedades, TeX, CSS y preprocesadores, JavaScript/TypeScript, Python, Go, Rust, Java/Kotlin, C/C++, C#, Swift, Dart, Ruby, shell/PowerShell/batch, SQL, GraphQL y otros equivalentes listados en el clasificador.

Office binario (`docx`, `xlsx`, `pptx`, etc.) no se anuncia como visualizable todavía porque no existe un renderer local real para esos formatos; se mantiene la regla de no mostrar un visor falso.

## Compatibilidad/offline

No se añadió CDN, npm ni servicio externo. El componente, CSS y JavaScript continúan embebidos en el binario Go. Los originales y previews pasan por las rutas autenticadas del servidor/VFS.
