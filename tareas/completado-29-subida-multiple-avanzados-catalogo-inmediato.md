# Completado 29: subida múltiple y catálogo inmediato

Fecha: 2026-08-15
Estado: completado

## Entregado

- **Nuevo → Subir archivos** simplificado a drag & drop/selector como vista predeterminada.
- Selector de archivos con `multiple` y resumen de cantidad/tamaño.
- Unidad, carpeta y ruta movidas a **Avanzados**, cerrado por defecto.
- Restablecimiento seguro del destino al cerrar el diálogo.
- Backend multipart capaz de procesar hasta 100 archivos por tanda mediante streaming.
- Resultado parcial para lotes con archivos exitosos y fallidos.
- Drag & drop de Mi unidad reutilizando la subida multipart por lotes.
- Inserción inmediata del archivo recién escrito en el catálogo para que aparezca en Mi unidad sin esperar el scan completo.
- Reindexación posterior conservada para thumbnails, previews, dimensiones e integridad.
- README y pruebas actualizados para el nuevo flujo.
