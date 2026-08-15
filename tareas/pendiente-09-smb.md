# Tarea 09 — SMB

Reevaluar una implementación SMB de servidor en Go cuando WebDAV haya sido probado con clientes reales.

## Investigación 2026-08-15

Hay implementaciones de servidor SMB2/3 en Go, pero las opciones maduras siguen teniendo compromisos importantes. Una alternativa reciente con licencia MIT afirma soportar SMB2/3, signing, encryption y VFS, pero todavía tiene adopción mínima y no ofrece releases; otra implementación con mayor adopción usa AGPL/comercial y reconoce limitaciones en locking/autenticación/conexiones paralelas.

Por seguridad y por la regla Ponytail, SMB sigue pendiente hasta tener una biblioteca suficientemente probada para exponer el VFS con los mismos usuarios sin degradar autenticación, locking ni licencia del proyecto.
