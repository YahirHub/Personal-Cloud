# Tarea 06 — Despliegue, TLS y hardening

## Objetivo

Dejar una base segura para ejecutar Personal Cloud como servicio local o detrás de un proxy HTTPS.

## Alcance completado

- TLS directo opcional mediante certificado y clave PEM.
- Redirección HTTPS opcional y HSTS cuando HTTPS es obligatorio.
- Cookies `Secure` forzadas si HTTPS obligatorio o TLS directo están activos.
- `X-Forwarded-For` y `X-Forwarded-Proto` solo desde proxies confiables.
- Healthcheck `GET /salud`.
- Cabeceras CSP, nosniff, frame denial, referrer y permissions policy.
- Backup diario de metadatos a `data/backups/`, retención de 7 copias.
- Backup incluye `state.json` y snapshot consistente del catálogo; no duplica originales ni caché regenerable.
- Ejemplo Caddy.
- Unidad systemd con usuario dedicado, sandbox y capacidad de montaje limitada al proceso.
- Instalador systemd de referencia.
- Instalador/desinstalador de tarea de inicio de Windows con privilegios elevados para las operaciones de volumen.
- Apagado limpio por SIGINT/SIGTERM.

## Riesgo conocido

En Linux montar filesystems requiere `CAP_SYS_ADMIN`, una capacidad amplia. El servicio se ejecuta como usuario dedicado y con sandbox, pero si el modelo de amenazas crece convendrá separar las operaciones privilegiadas en un helper mínimo. No se introduce ese helper ahora por YAGNI.
