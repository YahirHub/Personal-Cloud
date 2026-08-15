# 16 — Acciones Drive, Recientes, Destacados y filtros

## Objetivo

Acercar la experiencia diaria a Google Drive sin añadir controles decorativos. Las funciones nuevas deben operar sobre el VFS, el catálogo y el estado persistente real del usuario.

## Navegación y creación

El botón **Nuevo** de la barra lateral abre un menú compacto con acciones reales:

- **Nueva carpeta**: crea la carpeta en la unidad/ruta actual mediante el VFS. Desde una vista global permite seleccionar una unidad online y escribible.
- **Subir archivo**: reutiliza el flujo de subida existente y conserva la carpeta actual como destino.

Dentro de Mi unidad también se acepta arrastrar y soltar archivos sobre la página. El overlay solamente se habilita cuando existe un destino escribible. Los archivos se envían uno por uno al endpoint de subida ya protegido por sesión/CSRF y al finalizar se vuelve a cargar la carpeta para reflejar la indexación.

## Recientes y Destacados

Se añadieron rutas reales:

- `/recientes`: archivos del catálogo ordenados por modificación descendente;
- `/destacados`: archivos marcados por el usuario actual.

Los destacados se persisten en el estado versión 4 mediante pares `user_id + file_id`. Cuando un archivo se mueve o renombra desde Personal Cloud, la marca se migra al nuevo ID estable del catálogo. Si el archivo se elimina, la marca también se elimina.

La migración v3 → v4 es compatible con instalaciones existentes. Además se corrigió `cloneState` para que las mutaciones de estado conserven siempre `AppSettings` y no reinicien preferencias no relacionadas.

## Menú de archivo

El menú contextual de archivo contiene acciones funcionales equivalentes a las más útiles de Drive:

- Abrir;
- Descargar;
- Información;
- Agregar/Quitar de Destacados;
- Renombrar;
- Mover;
- Eliminar.

**Renombrar** modifica el original mediante VFS, comprueba colisiones, actualiza ruta/nombre/fecha del catálogo, mueve la caché asociada y conserva el destacado. Las acciones que necesitan el original se deshabilitan si la unidad está offline; Información y estado de Destacados siguen disponibles desde metadatos locales.

## Filtros

Las vistas de archivos incorporan filtros reales, con diseño oscuro tipo Drive:

- **Tipo**: imágenes, video, audio, documentos, comprimidos u otros;
- **Modificado**: últimas 24 horas, 7 días, 30 días o un año;
- **Fuente**: unidad virtual, disponible en búsqueda/Recientes/Destacados donde existen varias fuentes.

Al activar filtros se muestran únicamente archivos, no directorios derivados, porque el filtro describe propiedades de archivo. Los parámetros se propagan al endpoint de scroll infinito y a los enlaces de paginación/modo de listado para evitar resultados inconsistentes al continuar navegando.

No se agregó un filtro **Personas** porque el modelo actual no implementa propietarios/colaboración por archivo. La regla se mantiene: una característica visual de Drive solo aparece si Personal Cloud puede respaldarla con comportamiento real.

## Página principal

La Página principal conserva la composición de referencia: bienvenida, **Carpetas sugeridas**, **Archivos sugeridos**, selector lista/cuadrícula y tarjetas con preview. Los `⋯` de carpetas-unidad y archivos apuntan a menús funcionales; no se renderizan puntos decorativos.
