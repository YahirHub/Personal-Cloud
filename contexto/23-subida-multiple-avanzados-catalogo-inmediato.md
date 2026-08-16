# 23. Subida múltiple, opciones avanzadas y catálogo inmediato

Fecha: 2026-08-15

## Objetivo

Simplificar **Nuevo → Subir archivos** para que el flujo normal se parezca a Drive: primero se eligen o arrastran archivos y las decisiones de unidad/carpeta sólo aparecen cuando el usuario abre **Avanzados**. Al mismo tiempo se corrige la latencia que hacía que un archivo recién subido pudiera no aparecer todavía en **Mi unidad** mientras la indexación completa seguía pendiente.

## Diálogo de subida

- El estado inicial sólo muestra un área grande de drag & drop, **Seleccionar archivos**, el límite por archivo y el botón de subida.
- El `input[type=file]` usa `multiple` y la tanda admite hasta 100 archivos.
- La selección enseña cantidad, tamaño total y una lista compacta de nombres.
- **Avanzados** está colapsado por defecto. Al abrirlo aparecen unidad, navegador de carpetas y ruta relativa.
- Cerrar el diálogo restablece el destino automático/actual para evitar que una ubicación manual anterior quede oculta y se reutilice accidentalmente.
- Los campos canónicos `destination_root` y `target_dir` se mantienen antes de los streams multipart. Los controles visibles sincronizan esos campos ocultos para que el backend conozca el destino antes de empezar a leer archivos grandes.

## Subida por lotes

`POST /archivos/subir` ya no termina después del primer `file` multipart:

- procesa secuencialmente hasta 100 archivos sin cargarlos completos en RAM;
- mantiene `MAX_UPLOAD_BYTES` como límite individual por archivo;
- limita también el tamaño máximo posible de la petición multipart completa;
- permite que una tanda automática distribuya archivos distintos en unidades diferentes según las reglas existentes;
- encola como máximo una reconciliación por unidad afectada;
- devuelve resultado parcial cuando unos archivos terminan y otros fallan, en vez de ocultar las subidas que sí se completaron.

El drag & drop realizado directamente sobre Mi unidad/carpeta también envía una sola tanda multipart, reutilizando exactamente el mismo endpoint.

## Corrección de archivos recién subidos que no aparecían

Antes, después de `WriteAtomic`, el servidor sólo llamaba `Indexer.Enqueue(storageID)`. En una unidad grande el worker podía tardar en contar/recorrer el medio y, durante ese intervalo, `Mi unidad` seguía leyendo un catálogo que aún no conocía el nuevo archivo.

Ahora, inmediatamente después de confirmar la escritura física:

1. el VFS hace `Stat` de la ruta virtual recién creada;
2. se calcula el mismo ID estable `storageID + relativePath` que utiliza el indexador;
3. se inserta una entrada mínima mediante `Catalog.UpsertBatch` con nombre, tipo, MIME, tamaño, fecha, unidad y ruta relativa;
4. Mi unidad puede mostrar el archivo en la siguiente respuesta sin esperar al escaneo;
5. el indexador sigue ejecutándose en segundo plano y completa thumbnail, preview, dimensiones, salud e integridad.

No se crea un segundo catálogo ni una caché paralela: la actualización inmediata y el indexador convergen sobre el mismo ID estable.

## Seguridad y límites

- CSRF se valida antes de aceptar el primer stream de archivo.
- Las rutas manuales siguen pasando por las mismas validaciones de VFS y política de categoría.
- Cada archivo se escribe de forma atómica mediante temporal + sync + rename.
- Un lote no supera 100 archivos; el frontend toma ese valor del backend para evitar divergencias.
- No se relaja ninguna política CSP ni se añade dependencia frontend/CDN.
