# Completado 31 — Login detrás de proxy HTTPS

## Objetivo

Corregir el inicio de sesión cuando Personal Cloud se publica mediante un túnel/reverse proxy que termina TLS antes del backend HTTP.

## Correcciones

- Separar la detección de HTTPS del código WebDAV.
- Aceptar `X-Forwarded-Proto`, `Forwarded` y `CF-Visitor` únicamente desde proxies confiables.
- Mantener `Secure` en las cookies cuando `APP_REQUIRE_HTTPS=true`.
- Añadir diagnóstico seguro de esquema/proxy al log HTTP.
- Añadir pruebas negativas contra spoofing de forwarded headers.
- Añadir regresión de login por proxy con cookie de sesión `Secure`.

## Verificación

- `go test ./...` ✅
- `go vet ./...` ✅

## Configuración recomendada para túnel local

```text
APP_ADDR=127.0.0.1:8736
APP_REQUIRE_HTTPS=true
APP_COOKIE_SECURE=false
APP_TRUSTED_PROXIES=127.0.0.1/32,::1/128
```

El servidor fuerza internamente `CookieSecure=true` porque HTTPS es obligatorio.
