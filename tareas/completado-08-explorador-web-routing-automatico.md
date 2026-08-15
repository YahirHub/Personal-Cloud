# Tarea 08 — Explorador web y routing automático

## Estado

Completado en R6.

## Implementado

- Explorador `/archivos` sobre el catálogo persistente.
- Navegación por carpetas derivadas del índice sin montar discos para listar.
- Descarga de originales mediante VFS y montaje bajo demanda.
- Subida desde la raíz con selección automática de unidad por tipo/categoría.
- Desempate por espacio libre conocido entre unidades equivalentes.
- Subida dentro de una raíz virtual a la unidad explícita correspondiente.
- Reindexación automática después de cada subida.
- Corrección del flujo inicial de almacenamiento: registrar una unidad inicia su primera indexación.
- Botón `Indexar ahora` visible en unidades registradas.
- Estado de indexación visible y actualización automática mientras está en cola/escaneando.
- CTA desde Fotos hacia Almacenamiento cuando el catálogo está vacío.

## Límites deliberados

- Los directorios vacíos no se guardan todavía en el catálogo; el explorador deriva carpetas de archivos indexados.
- El espacio libre solo puede usarse como criterio cuando el sistema lo conoce sin montar otra unidad.
- No se agregó búsqueda global todavía; no es necesaria para cerrar el flujo base.
