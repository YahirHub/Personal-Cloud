# Tarea 15: integridad, sincronización y operaciones masivas

## Objetivo
Completar funciones de nube personal para detectar medios dañados, reconciliar cambios externos, mover/eliminar/descargar múltiples archivos de forma segura y eficiente, agregar sincronización manual/periódica y mejorar acciones táctiles/selección en Galería y Archivos.

## Alcance
- Detectar medios dañados durante indexación y mostrar recomendación de eliminación.
- Omitir aviso o eliminar dañados por unidad.
- Reconciliar archivos eliminados/agregados fuera del panel mediante sincronización.
- Configuración de sincronización periódica global, desactivada por defecto.
- Movimiento de uno o varios archivos entre unidades/carpetas sin reindexación completa.
- Crear carpeta de destino al mover.
- Selección múltiple con ZIP streaming de bajo consumo, mover y eliminar con confirmación.
- Clic derecho y pulsación larga para acciones.
- Pulir filtro/orden con iconografía offline.
- Mantener seguridad de rutas, CSRF, autorización, límites y descargas opacas.

## Estado final
Completado y cubierto por pruebas unitarias/integración. La validación de corrupción es conservadora: `damaged` solo cuando existe evidencia de archivo ilegible; los formatos que no pueden verificarse quedan `unchecked`. La prueba definitiva de montaje/desconexión y movimiento entre medios físicos depende del hardware del usuario.
