# Fecha

2026-08-15

# Objetivo

Completar la primera capa útil de almacenamiento de Personal Cloud: descubrir y registrar unidades en Windows/Linux, presentarlas mediante un VFS, montarlas bajo demanda, desmontarlas por inactividad, mantener un catálogo local con thumbnails/previews y exponer el namespace por WebDAV usando la misma cuenta del servidor.

# Decisiones tomadas

- Se mantiene un único binario Go y cero módulos externos.
- El estado pequeño de usuarios/sesiones/unidades permanece en `data/state.json` (schema v2).
- La migración v1 -> v2 agrega `volumes` sin eliminar usuarios, sesiones ni onboarding existentes.
- Cada unidad se identifica por un ID persistente del SO: UUID en Linux y Volume GUID en Windows.
- No se filtra únicamente por el indicador `removable`, porque HDD/SSD USB reales pueden presentarse como discos fijos. Se excluyen recursos del sistema y el administrador registra explícitamente los volúmenes de datos.
- El VFS es la única capa que traduce `/RaizVirtual/ruta` a una ubicación física. Web, catálogo y WebDAV no conocen letras de unidad ni `/dev/*`.
- Las operaciones obtienen un lease. Mientras exista al menos un lease, la unidad no puede auto-desmontarse.
- Mount/unmount se serializa por unidad para evitar carreras entre solicitudes concurrentes.
- Auto-desmontaje se revisa cada 10 segundos y solo procede con `active == 0` y timeout vencido.
- Windows nunca fuerza el desmontaje: flush -> lock -> dismount -> retirar punto de montaje. Si no puede bloquear, devuelve error.
- Linux usa mount/unmount del kernel cuando el filesystem es conocido y delega al helper `mount` únicamente para drivers/formats que el SO maneja mejor.
- El catálogo usa snapshot + journal JSONL y un mapa en memoria. Se eligió así para conservar cero dependencias durante la etapa personal/mononodo. Está marcada deuda técnica para migrar a índice disk-backed si escala a ~500k archivos o la memoria/arranque lo exige.
- El indexador es deliberadamente de un solo worker para no despertar/golpear varios HDD a la vez.
- Miniaturas y previews viven en el almacenamiento interno y son privadas; verlas no monta la unidad original.
- El original solo se abre mediante el VFS, que monta su unidad y mantiene el lease durante el stream.
- Thumbnails actuales se generan con la biblioteca estándar para JPEG, PNG y GIF. Fuentes de más de 80 MP se catalogan pero no se decodifican para limitar picos de memoria. Otros formatos de imagen se catalogan pero su decoder queda para la tarea 07.
- WebDAV se implementa con `net/http` para no introducir una dependencia por el protocolo. Usa las mismas credenciales y el mismo VFS. Sus mutaciones notifican al indexador; si cambian archivos durante un scan activo, se marca una segunda pasada para no perder reconciliación.
- Basic Auth WebDAV requiere HTTPS fuera de loopback por defecto.
- Una autenticación WebDAV válida se cachea 5 minutos mediante una clave HMAC secreta en memoria, evitando recalcular PBKDF2 en cada PROPFIND/GET sin guardar la contraseña.
- TLS puede terminar en el propio binario o en un proxy confiable.
- Se crea backup diario local de metadatos con 7 copias: `state.json` + snapshot consistente del catálogo. No copia originales ni thumbnails/previews regenerables.

# Arquitectura actual

```text
HTTP/HTTPS
   |
   +-- Web UI -----------------------+
   |                                 |
   +-- WebDAV -> auth/rate limit ----+----> VFS
                                             |
                       +---------------------+---------------------+
                       |                                           |
                Storage Manager                              Catalog
                       |                                  snapshot+journal
             lease / mount / idle                    thumbnails / previews
                       |
          +------------+------------+
          |                         |
     unidad A                    unidad B
   Documentos                   Multimedia
```

Flujo de una foto:

```text
indexar unidad
 -> lease (monta si hace falta)
 -> escaneo
 -> metadata + thumbnail + preview en SSD interno
 -> release
 -> timeout
 -> desmontaje

GET /fotos
 -> catálogo/cache únicamente

GET /archivos/<id>/original
 -> catálogo resuelve storage_id + ruta
 -> VFS obtiene lease
 -> monta unidad si está desmontada
 -> stream
 -> release
 -> timeout
 -> desmontaje
```

# Librerías usadas

Solo biblioteca estándar de Go.

Nuevos grupos principales:

- almacenamiento: `os`, `syscall`, `os/exec`, Win32 vía `syscall.NewLazyDLL`.
- VFS: `os`, `io`, `path/filepath`.
- catálogo: `encoding/json`, `image`, `image/jpeg`, `image/png`, `image/gif`.
- WebDAV: `net/http`, `encoding/xml`.
- backup: `archive/zip`.
- TLS: `net/http` / `crypto/tls` de forma indirecta mediante `ListenAndServeTLS`.

# Archivos importantes modificados

- `cmd/server/main.go`
- `internal/app/*`
- `internal/backup/*`
- `internal/catalog/*`
- `internal/config/*`
- `internal/storage/*`
- `internal/store/*`
- `internal/vfs/*`
- `internal/webdav/*`
- `internal/webui/*`
- `web/pages/*`
- `web/static/app.css`
- `deploy/linux/*`
- `deploy/windows/*`
- `deploy/caddy/*`
- `.env.example`
- `README.md`
- `tareas/*`

# Problemas encontrados

- Dos solicitudes simultáneas sobre una unidad desmontada podían intentar montarla al mismo tiempo si solo se protegía el contador de handles.
- Un HDD/SSD conectado por USB no necesariamente aparece como `removable` en Windows/Linux.
- Recalcular PBKDF2 para cada solicitud WebDAV válida genera CPU innecesaria porque los clientes hacen muchos PROPFIND/GET consecutivos.
- Una foto modificada conservando la misma ruta podría reutilizar una miniatura vieja si solo se usaba un ID derivado de ruta.
- El servidor HTTP no debe imponer un `ReadTimeout`/`WriteTimeout` corto que corte uploads/downloads grandes.
- Desmontar un volumen Windows que otro proceso está usando puede provocar problemas; no debe hacerse a la fuerza.

# Soluciones implementadas

- Mutex de operación por volumen para serializar mount/unmount.
- Leases y contador de operaciones activas.
- Detección por identidad persistente y selección explícita del administrador.
- Caché HMAC de autenticación WebDAV válida, TTL 5 minutos, invalidada por cambio de hash del usuario.
- Regeneración de caché de imagen cuando cambian `size` o `mtime`.
- `ReadHeaderTimeout` protege headers, mientras las transferencias grandes no tienen un timeout global arbitrario.
- Windows exige lock exitoso antes del dismount.
- Tests unitarios para VFS, catálogo, WebDAV, almacenamiento, migraciones, backup, configuración y autenticación.
- Cross-build Windows/amd64 y build Linux sin CGO.

# Pendientes

1. Validar detección/montaje/desmontaje sobre discos físicos reales en el Windows del MiniPC/PC y en el Linux de producción.
2. Añadir decoders/portadas para formatos multimedia avanzados (tarea 07).
3. Crear explorador web general y routing automático de uploads (tarea 08).
4. Reevaluar SMB después de validar WebDAV con clientes reales (tarea 09).
5. Si el catálogo crece hasta un punto donde el mapa en memoria no sea razonable, migrarlo a un índice disk-backed manteniendo la API del paquete `catalog`.
6. Un backup local de metadatos no protege contra fallo físico del SSD interno; un destino secundario/off-site será una fase posterior.

# Próximos pasos

Probar R5 sobre hardware real: registrar una memoria/HDD, indexarla, comprobar que la galería continúa disponible tras auto-desmontaje y abrir un original para verificar el montaje bajo demanda. Después priorizar tarea 07 u 08 según el uso real.
