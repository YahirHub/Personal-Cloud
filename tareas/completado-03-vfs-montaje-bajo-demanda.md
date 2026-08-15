# Tarea 03 — VFS y montaje bajo demanda

## Objetivo

Presentar las unidades registradas como un namespace virtual único y mantener los discos montados únicamente mientras exista una operación real o hasta expirar su timeout de inactividad.

## Alcance completado

- Raíces virtuales independientes de letras de unidad y `/dev/*`.
- Leases por operación con contador de handles activos.
- Montaje bajo demanda.
- Auto-desmontaje configurable desde 30 segundos hasta 7 días.
- Serialización de mount/unmount por volumen para impedir carreras entre accesos concurrentes.
- Desmontaje rechazado mientras haya operaciones activas.
- Intento de desmontaje limpio durante el cierre del servidor para unidades con auto-desmontaje.
- Linux: `mount`/`umount` nativo, usando helper del SO solo cuando el filesystem lo requiere.
- Windows: `FlushFileBuffers`, lock del volumen, dismount y retirada del mount point; nunca se fuerza si el volumen no puede bloquearse.
- Lecturas y escrituras protegidas contra path traversal y symlinks que escapen de la raíz física.
- Escrituras temporales + sync + reemplazo para evitar archivos finales parciales.
- Políticas de tipo de archivo por categoría.
