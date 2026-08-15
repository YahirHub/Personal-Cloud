# 15 — Paridad Drive, reconexión y miniaturas orientadas

## Objetivo

Pulir la experiencia introducida en la interfaz Drive oscura sin convertir controles visuales en elementos decorativos. La página principal debe conservar la composición familiar de Google Drive, mientras que las acciones visibles deben operar sobre funciones reales del servidor.

## Página principal y Mi unidad

La página principal mantiene dos bloques principales: **Carpetas sugeridas** y **Archivos sugeridos**. Los archivos admiten cuadrícula y lista persistentes; la lista expone columnas de nombre, motivo sugerido, propietario y ubicación para acercarse a la composición de Drive sin inventar colaboración o recursos que el servidor no posee.

Las unidades de `Mi unidad` ya no renderizan un `⋯` decorativo. Cada unidad expone un menú contextual real con:

- abrir la raíz virtual;
- consultar información/propiedades en tiempo real;
- actualizar el catálogo;
- montar/conectar la unidad cuando está disponible;
- abrir la administración completa de almacenamiento.

Las propiedades se consultan mediante API y muestran estado, capacidad, espacio libre, modo lectura/escritura, filesystem, estadísticas del catálogo y estado de indexación. Los identificadores técnicos quedan dentro de un detalle desplegable para no ensuciar la experiencia normal.

Los archivos también exponen **Información** además de Abrir, Descargar, Mover y Eliminar. Si una unidad está desconectada, las acciones que requieren el original se deshabilitan, pero la información del catálogo continúa accesible.

## Reconexión de unidades

La identidad primaria sigue siendo Volume GUID en Windows y UUID/PARTUUID en Linux. Sin embargo, algunos medios extraíbles pueden reaparecer con una identidad primaria distinta después de una reconexión.

La resolución ahora usa este orden:

1. coincidencia exacta por identidad persistente;
2. si no existe, coincidencia por `HardwareID` solamente cuando es inequívoca;
3. si varias particiones comparten hardware, se exige además una coincidencia única de etiqueta/filesystem;
4. al recuperar la unidad por hardware, la nueva identidad persistente y los datos de dispositivo se guardan atómicamente;
5. las aperturas hacen una segunda detección breve tras 350 ms para cubrir el periodo inmediatamente posterior a conectar físicamente el disco.

Nunca se toma una coincidencia de hardware ambigua: es preferible mantener una unidad offline a enlazarla con el medio equivocado.

La detección ligera de presencia incorpora el identificador de hardware y cuenta cuántos volúmenes lo comparten, de modo que Galería y las vistas de disponibilidad también reconocen una reconexión segura sin tener que montar la unidad.

## Orientación de miniaturas

El visualizador del original delega la orientación a la imagen original, mientras que las miniaturas/previews son JPEG generados localmente. Por eso una caché antigua podía seguir viéndose girada aun cuando el original se mostrara correctamente.

Se incrementó `ImageCacheVersion` a 3 y se añadió regeneración perezosa:

- las URLs de caché llevan versión para romper cachés anteriores del navegador;
- al pedir una miniatura/preview de imagen con versión antigua, el indexador intenta regenerarla desde el original aplicando EXIF Orientation;
- si la unidad está desconectada, se puede seguir sirviendo la caché previa, pero con `no-cache` para que el navegador vuelva a validar cuando se reconecte;
- al reconectar, el primer acceso actualiza thumbnail, preview, dimensiones y versión en el catálogo;
- las transformaciones EXIF 1–8 quedan cubiertas por pruebas.

Esto evita obligar a ejecutar una reindexación completa únicamente para corregir la orientación visual.

## Regla de interfaz

No agregar iconos, `⋯`, toggles o botones que no tengan una acción real. Si una función equivalente de Google Drive no existe todavía en Personal Cloud, se omite hasta que tenga backend y manejo de errores reales.
