# Tarea 05 — WebDAV

## Objetivo

Exponer el VFS como servidor WebDAV usando las mismas credenciales de Personal Cloud y sin revelar rutas físicas.

## Alcance completado

- Endpoint `/webdav/`.
- Basic Auth únicamente aceptable fuera de loopback cuando la petición se considera HTTPS.
- Misma base de usuarios y mismo hash de contraseña que la interfaz web.
- Rate limit por IP y por combinación IP/usuario antes del PBKDF2.
- Caché de autenticación positiva de 5 minutos en memoria mediante HMAC; no persiste contraseñas y se invalida si cambia el hash del usuario.
- Métodos: OPTIONS, PROPFIND depth 0/1, GET, HEAD, PUT, DELETE, MKCOL, MOVE, COPY de archivo, LOCK y UNLOCK.
- Locking exclusivo en memoria con expiración.
- Límite global de upload.
- VFS aplica las mismas categorías, solo lectura y protecciones de path traversal.
- PUT/DELETE/MKCOL/MOVE/COPY notifican al indexador para reconciliar el catálogo; si una mutación ocurre durante un scan, se agenda automáticamente una segunda pasada.

## Límite deliberado

`COPY` recursivo de directorios no se implementa todavía. Está marcado con `deuda-tecnica:` y debe añadirse si un cliente real utilizado por el proyecto lo necesita.
