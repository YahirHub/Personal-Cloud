# Fecha

2026-08-25

# Problema

El login podía parecer un simple “recarga de la página” al publicar Personal Cloud detrás de un túnel/reverse proxy que terminaba TLS. El backend escucha HTTP localmente, por lo que `r.TLS` es `nil` aunque el navegador esté usando HTTPS.

# Causa técnica

La detección de HTTPS estaba centralizada dentro del código WebDAV y únicamente aceptaba `X-Forwarded-Proto`. Si el proxy utilizado no entregaba ese encabezado exactamente en esa forma, `APP_REQUIRE_HTTPS=true` consideraba que la petición era HTTP y devolvía un `308` hacia la misma URL HTTPS. En un túnel HTTPS esto puede verse en el navegador como una recarga o bucle.

Además, el diagnóstico anterior de sesiones inválidas no distinguía con claridad entre una cookie que nunca se emitió, una cookie que el navegador no volvió a enviar y una sesión que no pudo resolverse en el store.

# Corrección

- `requestIsHTTPS` vive ahora en `internal/app/proxy.go`, separado de WebDAV.
- Una petición directa con `r.TLS != nil` sigue considerándose HTTPS.
- Cuando TLS termina en un proxy, `X-Forwarded-Proto`, `Forwarded: proto=...` y `CF-Visitor: {"scheme":"..."}` solo se aceptan si el peer inmediato pertenece a `APP_TRUSTED_PROXIES`.
- `X-Forwarded-Proto` conserva prioridad y se interpreta únicamente con el primer valor de una cadena de proxies.
- Un peer no confiable no puede convertir una petición HTTP en HTTPS mediante headers falsificados.
- El log HTTP incluye `https`, `remote_addr` y `x_forwarded_proto` para comprobar el comportamiento real del túnel sin registrar tokens de sesión.
- Se añadió una prueba de login detrás de un proxy HTTPS confiable que comprueba que se emite la cookie `pc_session` con `Secure`.
- Se añadieron pruebas para `X-Forwarded-Proto`, `Forwarded` y `CF-Visitor`, además de un caso negativo para un peer no confiable.

# Configuración del túnel

Para un túnel local que termina TLS en el proveedor y conecta con el servidor por HTTP, se mantiene:

```text
APP_REQUIRE_HTTPS=true
APP_COOKIE_SECURE=false
```

`APP_COOKIE_SECURE=false` no desactiva `Secure` cuando HTTPS es obligatorio: `config.Load` lo fuerza a `true`. La recomendación sigue siendo no desactivar `APP_REQUIRE_HTTPS` para “arreglar” el login.

`APP_TRUSTED_PROXIES=*` permite confiar en headers reenviados desde cualquier peer y solo debe usarse cuando el puerto backend no sea accesible por otros clientes. Para un túnel que corre en la misma máquina es preferible restringirlo a loopback:

```text
APP_TRUSTED_PROXIES=127.0.0.1/32,::1/128
```

# Resultado esperado

Con una URL HTTPS pública del túnel:

```text
navegador HTTPS
    -> proxy/túnel
    -> HTTP local :8736
    -> X-Forwarded-Proto: https
    -> requestIsHTTPS=true
    -> login POST
    -> Set-Cookie: pc_session; Secure; HttpOnly; SameSite=Lax
    -> 303 /inicio o /bienvenida
```

La sesión sigue almacenando únicamente el hash SHA-256 del token y no se modifica el modelo de autenticación.

# Ajuste posterior — cookie emitida en respuesta 200

Los intentos con 303 y 302 seguían llegando a `/bienvenida` sin `pc_session`, mientras `pc_csrf` sí permanecía en el navegador. La regresión de backend demostraba que `pc_session` se generaba correctamente, por lo que se aisló el problema en el transporte de la respuesta de login mediante el intermediario.

El login exitoso ya no depende de transportar `Set-Cookie` dentro de una respuesta 3xx. Después de crear la sesión, responde `200 OK`, conserva `Set-Cookie: pc_session` y envía `Refresh: 0; url=...` más un enlace manual de respaldo. La respuesta lleva `Cache-Control: no-store` y `X-PC-Login-Established: 1` para facilitar la verificación del túnel sin revelar el token.

La cookie mantiene `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/` y `Max-Age` según `APP_SESSION_TTL`.
