# Personal Cloud

Servidor de almacenamiento personal escrito en Go. Esta primera etapa implementa el núcleo de configuración, autenticación y onboarding sobre el que se conectarán el gestor de unidades, el almacenamiento virtual, el catálogo de miniaturas y WebDAV.

## Estado actual

Implementado:

- Primer arranque protegido mediante `/setup`.
- Código de configuración aleatorio generado en el log mientras no exista administrador.
- Creación exclusiva de la primera cuenta administradora.
- Contraseñas almacenadas con PBKDF2-HMAC-SHA256, salt aleatorio y formato versionado.
- Sesiones persistentes con token aleatorio; solo se almacena su hash SHA-256.
- Protección CSRF en formularios mutables.
- Rate limit reutilizable para formularios sensibles.
- Límite doble de login por IP y por usuario.
- Manejo seguro de `X-Forwarded-For` únicamente desde proxies configurados como confiables.
- Onboarding inicial en `/bienvenida`.
- Rutas amigables en español.
- Layout y componentes HTML reutilizables.
- Recursos web embebidos en el binario.
- Estado de usuarios y sesiones persistido con archivos JSON privados y reemplazo seguro con backup.
- Auditoría append-only en JSON Lines con retención automática de 90 días y tope de 50,000 registros.
- Cabeceras de seguridad y CSP sin dependencias frontend externas.
- Vistas base `/inicio`, `/almacenamiento` y `/fotos` preparadas para los siguientes módulos.

Pendiente por fases: detección/montaje de unidades, VFS, indexación, thumbnails/previews, WebDAV y hardening de despliegue.

## Dependencias

El núcleo actual usa **únicamente la biblioteca estándar de Go**.

`go.mod` no requiere módulos externos. `go mod tidy` funciona directamente con una instalación normal de Go.

El proyecto declara Go 1.23 como versión mínima de lenguaje. Para builds de producción usa una versión de Go actualmente soportada y con los últimos parches de seguridad de su rama.

## Preparar y probar

```bash
go mod tidy
go test ./...
go vet ./...
```

En Windows CMD también puedes usar:

```bat
scripts\deps.cmd
scripts\test.cmd
```

## Ejecutar

```bash
go run ./cmd/server
```

En Windows CMD:

```bat
scripts\run.cmd
```

Por defecto escucha en `:8080` y crea:

```text
data/state.json
data/state.json.bak
data/audit.jsonl
```

En el primer arranque aparecerá en consola un mensaje similar a:

```text
CONFIGURACIÓN INICIAL REQUERIDA url=/setup codigo=ABCD-EFGH-JKLM
```

Abre:

```text
http://127.0.0.1:8080/setup
```

Introduce el código actual, usuario y una contraseña de al menos 12 caracteres. Tras crear el administrador el código se elimina de memoria y `/setup` deja de permitir el alta inicial.

## Configuración

Las variables de configuración de la aplicación están documentadas en `.env.example`.

| Variable | Predeterminado | Uso |
| --- | --- | --- |
| `APP_ADDR` | `:8080` | Dirección HTTP de escucha |
| `APP_DATA_DIR` | `./data` | Directorio privado para estado, auditoría y futuros metadatos |
| `APP_COOKIE_SECURE` | `false` | Debe ser `true` detrás de HTTPS en producción |
| `APP_TRUSTED_PROXIES` | loopback | CIDRs autorizados para aportar `X-Forwarded-For` |
| `APP_SESSION_TTL` | `168h` | Duración de sesión |

La aplicación no carga `.env` automáticamente. La configuración puede inyectarse desde el sistema, servicio o contenedor.

## Proxy y HTTPS

Para exponer el servidor fuera de la LAN:

1. Termina TLS en un proxy confiable.
2. Define `APP_COOKIE_SECURE=true` en el entorno del servicio.
3. Añade exclusivamente las redes del proxy a `APP_TRUSTED_PROXIES`.
4. No confíes globalmente en `X-Forwarded-For` desde Internet.
5. Restringe el puerto interno del servidor a la LAN, loopback o red privada del proxy.

## Rate limit inicial

- `/setup`: 5 intentos por IP cada 10 minutos.
- `/iniciar-sesion`: 12 intentos por IP cada 15 minutos.
- Login adicional: 6 intentos por nombre de usuario cada 15 minutos.

El limitador es un servicio interno reutilizable para formularios sensibles futuros.

## Persistencia

`state.json` guarda únicamente el estado pequeño y crítico del núcleo: usuarios y sesiones. Cada mutación se escribe primero en un archivo temporal del mismo directorio, se sincroniza, conserva la versión anterior como `state.json.bak` y después sustituye el estado activo.

La auditoría se escribe en `audit.jsonl` para no reescribir todo el estado por cada intento de login. La limpieza periódica aplica retención y límite de filas.

El catálogo masivo de archivos/fotos **no** se almacenará en este JSON. La tarea de catálogo elegirá un índice apropiado cuando exista esa necesidad real; SQLite sigue siendo un candidato, pero no se añade prematuramente al bootstrap.

## Contraseñas

Las contraseñas usan PBKDF2-HMAC-SHA256 con:

- salt aleatorio de 128 bits;
- 600,000 iteraciones;
- salida de 256 bits;
- comparación en tiempo constante;
- formato persistido que incluye algoritmo y factor de trabajo para permitir migraciones futuras;
- límites al factor de trabajo leído para evitar hashes manipulados que provoquen consumo excesivo de CPU.

Nunca se registran contraseñas ni tokens de sesión.

## Compilar

Linux estático:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/personalcloud ./cmd/server
```

Windows desde Windows:

```bat
go build -trimpath -ldflags="-s -w" -o bin\personalcloud.exe ./cmd/server
```

Cross-build de Windows desde Linux:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/personalcloud.exe ./cmd/server
```

## Prueba manual mínima

1. Arranca con un `data/` vacío y comprueba que `/` redirige a `/setup`.
2. Comprueba que un código incorrecto no crea el administrador.
3. Crea el administrador con el código mostrado en consola.
4. Comprueba que inicia sesión automáticamente y abre `/bienvenida`.
5. Completa el onboarding y verifica la redirección a `/inicio`.
6. Cierra sesión y confirma que `/inicio` vuelve a `/iniciar-sesion`.
7. Intenta credenciales incorrectas repetidamente y comprueba HTTP 429 al alcanzar el límite.
8. Reinicia el servidor y confirma que `/setup` ya no vuelve a habilitarse.
9. Verifica que la sesión válida persiste tras reiniciar mientras no haya expirado.
10. Comprueba que `data/state.json.bak` aparece después de las primeras mutaciones.

## Seguridad del setup

El código de setup aparece en el log por diseño, pero solo mientras no existe ningún administrador. Vive únicamente en memoria, cambia tras cada nuevo arranque previo al bootstrap y queda invalidado al crear el administrador.
