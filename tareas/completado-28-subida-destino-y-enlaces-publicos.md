# Completado 28: destino manual y enlaces públicos

- Añadido selector opcional de unidad y carpeta en **Nuevo → Subir archivo** sin eliminar el routing automático.
- Validado el destino manual en backend y creación segura de subcarpetas.
- Añadido modelo persistente de enlaces públicos con migración de estado v4 → v5.
- Implementados enlaces públicos, contraseña opcional, renovación y revocación.
- Añadida vista `/compartidos` con gestión individual y revocación global.
- Corregida la administración para que un admin gestione el `share_id` del propietario real.
- Añadidos botones Compartir al menú contextual, selección individual y visores multimedia/documentales.
- Implementado `/embed` y desbloqueo protegido compatible con iframes mediante ticket HMAC temporal.
- Implementado reproductor de video público con controles propios y calidad adaptativa/variantes FFmpeg.
- Conservación de enlaces al mover/renombrar y revocación automática al eliminar.
- Redacción de tokens públicos en logs y rate limits de gestión, contraseña y transcoding.
- Añadidas pruebas de persistencia/ciclo de vida, UI de subida, UI de compartir, embed público y tickets firmados.
