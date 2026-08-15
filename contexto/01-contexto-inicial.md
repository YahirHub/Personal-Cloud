# Fecha

2026-08-15

# Objetivo

Crear la base de una nube personal escrita en Go para un MiniPC que más adelante unificará múltiples medios extraíbles, mantendrá catálogo y miniaturas en almacenamiento interno y montará los originales bajo demanda.

Esta primera implementación cubre bootstrap seguro, autenticación, sesiones, rate limiting, onboarding, URLs amigables y frontend reutilizable.

# Decisiones tomadas

- Go es el único lenguaje del servidor; HTML/CSS son recursos embebidos.
- `go.mod` declara Go 1.23 como lenguaje mínimo y no fuerza un toolchain concreto.
- El núcleo bootstrap no usa dependencias externas: toda su funcionalidad se implementa con la biblioteca estándar.
- No usar framework HTTP: `net/http` y `ServeMux` son suficientes.
- No usar framework frontend, Node, npm ni CDN.
- `/setup` solo está activo mientras no exista una cuenta `admin`.
- El código de setup se genera aleatoriamente por arranque, solo vive en memoria y se muestra en log.
- Crear el primer administrador se serializa dentro del store para evitar carreras.
- Tras crear el administrador se crea una sesión y se redirige a `/bienvenida`.
- El onboarding se registra por usuario con `onboarding_completed`.
- Sesiones: cookie aleatoria de 256 bits; el store guarda únicamente SHA-256 del token.
- Contraseñas: PBKDF2-HMAC-SHA256, salt aleatorio de 128 bits, 600,000 iteraciones, salida de 256 bits y formato versionado.
- El parser de hashes limita el factor de trabajo aceptado para evitar DoS por datos manipulados.
- Estado pequeño del núcleo: `data/state.json`, con escritura temporal + sync + backup previo `state.json.bak`.
- Auditoría: `data/audit.jsonl`, append-only, retención de 90 días y máximo 50,000 registros.
- El catálogo masivo de archivos no irá en `state.json`; se escogerá su índice en la tarea 04 cuando exista la carga real.
- CSRF usa cookie aleatoria HttpOnly + campo oculto comparado en tiempo constante.
- Rate limit inicial en memoria porque un único proceso es el objetivo actual. Si se escala horizontalmente deberá migrarse a almacenamiento compartido.
- Rate limit de setup por IP: 5/10 min.
- Rate limit de login por IP: 12/15 min y por usuario: 6/15 min.
- `X-Forwarded-For` solo se respeta cuando `RemoteAddr` pertenece a un CIDR configurado como proxy confiable.
- Rutas amigables canónicas: `/iniciar-sesion`, `/bienvenida`, `/inicio`, `/almacenamiento`, `/fotos`; `/setup` se conserva por requisito de bootstrap.
- Los templates usan layout y componentes reutilizables mediante `html/template`.
- Los recursos se embeben en el binario con `embed`.
- El diseño visual actual es funcional y sencillo; el pulido de UI será incremental.

# Arquitectura actual

```text
cmd/server
    -> config
    -> store (stdlib, estado JSON + auditoría JSONL)
    -> app
        -> auth (stdlib, PBKDF2-HMAC-SHA256)
        -> ratelimit
        -> webui
            -> web/assets embebidos
```

Flujo de primer arranque:

```text
servidor inicia
  -> consulta si existe admin
  -> si no existe: genera setup code y lo registra en consola
  -> GET / redirige /setup
  -> POST /setup valida CSRF + rate limit + setup code + credenciales
  -> crea admin de forma serializada y persistente
  -> crea sesión
  -> invalida setup code en memoria
  -> /bienvenida
  -> completar onboarding
  -> /inicio
```

# Librerías usadas

Solo biblioteca estándar de Go:

- `net/http`, `html/template`, `embed`.
- `crypto/rand`, `crypto/hmac`, `crypto/sha256`, `crypto/subtle`.
- `encoding/json` y utilidades estándar de archivos.
- `log/slog`, `context`, `sync`, `time`.

No hay módulos externos en `go.mod`.

# Archivos importantes modificados

- `cmd/server/main.go`
- `internal/app/*`
- `internal/auth/*`
- `internal/config/*`
- `internal/ratelimit/*`
- `internal/store/*`
- `internal/webui/renderer.go`
- `web/*`
- `scripts/*`
- `.env.example`
- `README.md`

# Problemas encontrados

- El estado pequeño de autenticación y sesiones no debe crecer hasta convertirse en el catálogo de archivos; son responsabilidades con perfiles de carga distintos.
- La persistencia debe tolerar un cierre durante una escritura sin dejar el servidor sin un estado recuperable.

# Soluciones implementadas

- `state.json` queda limitado al estado pequeño del núcleo. El catálogo masivo tendrá su propio índice en una tarea posterior.
- Cada cambio de estado se escribe en un temporal del mismo directorio, se sincroniza y conserva la versión anterior como `state.json.bak` antes del reemplazo.
- Si el estado activo falta al iniciar y existe un backup válido, el store lo restaura.
- La auditoría se escribe por separado en JSONL append-only para evitar reescribir todo el estado en cada evento.
- El núcleo usa exclusivamente la biblioteca estándar y mantiene un `go.mod` sin dependencias externas.
- `go mod tidy`, `go test ./...` y `go vet ./...` fueron ejecutados exitosamente.
- Se verificó compilación Linux y Windows/amd64 con `CGO_ENABLED=0`.

# Pendientes

1. Detección y registro de unidades extraíbles Linux/Windows.
2. Montaje/desmontaje por actividad.
3. Namespace virtual unificado.
4. Selección e implementación del índice de catálogo según escala real.
5. Catálogo de archivos y reconciliación.
6. Thumbnails/previews persistentes para imágenes y posteriormente video.
7. WebDAV sobre el VFS.
8. Despliegue endurecido detrás de proxy TLS.

# Próximos pasos

Implementar el gestor de unidades con identidad persistente, estado online/offline y políticas por tipo de contenido. Después conectar VFS e indexación para que la galería funcione sin mantener montados los discos.
