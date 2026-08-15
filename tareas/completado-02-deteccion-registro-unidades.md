# Tarea 02 — Detección y registro de unidades

## Objetivo

Detectar volúmenes locales aptos para almacenamiento en Linux y Windows, identificarlos de manera persistente y permitir registrarlos con una política propia.

## Alcance completado

- Linux: identidad por UUID desde `/dev/disk/by-uuid`.
- Windows: identidad por Volume GUID mediante Win32.
- Exclusión de la unidad del sistema y montajes Linux críticos.
- No asumir que un disco USB aparecerá como `removable`: se muestran volúmenes locales aptos y el administrador elige cuáles registrar.
- Estado registrado/no registrado, online/offline, montado/desmontado y en uso.
- Nombre, raíz virtual, categoría, timeout, auto-desmontaje y modo solo lectura.
- Migración automática del estado persistente v1 -> v2 sin perder usuarios ni sesiones.
- Identidad física refrescada solo cuando cambia.

## Verificación

- Tests Linux de parsing de `mountinfo` y protección de montajes críticos.
- Cross-build Windows/amd64.
- La operación física sobre hardware Windows/Linux debe verificarse en cada equipo real por depender de permisos y drivers del SO.
