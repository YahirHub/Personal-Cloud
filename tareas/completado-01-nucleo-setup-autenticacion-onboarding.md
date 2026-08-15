# Tarea 01 — Núcleo, setup, autenticación y onboarding

## Objetivo

Implementar el primer arranque seguro y la base web persistente del servidor.

## Alcance completado

- Código temporal de setup generado por arranque.
- Primera cuenta administradora exclusiva.
- PBKDF2-HMAC-SHA256 con salt aleatorio y formato versionado.
- Login/logout.
- Sesiones persistentes con token hasheado.
- Persistencia del estado crítico con biblioteca estándar.
- Backup del estado previo y recuperación si falta el archivo activo.
- Auditoría append-only con limpieza por retención/límite.
- CSRF.
- Rate limit reutilizable para formularios especiales.
- Onboarding.
- Rutas amigables.
- Layout/componentes reutilizables.
- Seguridad HTTP básica.
- Tests unitarios e integración del bootstrap.
- Build Linux y Windows/amd64 sin CGO.

## Verificación

Ejecutado correctamente:

```text
go mod tidy
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/server
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/server
```

No existen dependencias externas ni configuración especial del sistema de módulos.
