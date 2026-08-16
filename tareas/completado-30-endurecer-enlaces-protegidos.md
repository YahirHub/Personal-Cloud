# Completado 30 — Endurecer enlaces protegidos

Se corrigió el flujo de contraseña de enlaces públicos para que una autorización interna no pueda reutilizarse como URL pública independiente.

- URL pública limpia después del desbloqueo.
- Cookie HttpOnly de sesión para navegación normal.
- Grants HMAC v2 sólo para subrecursos de embeds con cookies de terceros bloqueadas.
- Validación de `Referer` del share/token exacto y `Sec-Fetch-Site` cuando está disponible.
- TTL máximo de 2 horas para grants de embed.
- `no-store` también para variantes de video protegidas.
- URLs antiguas con `access` en la página pública se limpian.
- Pruebas de regresión para copia de URL a otro contexto.

Prueba manual clave: desbloquear una imagen protegida, abrir/copiar la URL interna `/contenido?access=...` y pegarla en incógnito. Debe responder `401` o volver a exigir el flujo protegido; abrir `/s/<token>` en incógnito debe mostrar siempre la contraseña.
