# Personal Cloud

Servidor de almacenamiento personal mononodo escrito en Go para presentar varias unidades físicas como una nube lógica, mantener el catálogo visible aun cuando los discos de originales estén desmontados y exponer el mismo namespace por Web y WebDAV.

La implementación actual está orientada a un MiniPC con almacenamiento interno para metadatos/caché y HDD/SSD/USB externos para originales.

## Estado actual

Implementado:

- Bootstrap seguro en `/setup` mediante código temporal mostrado únicamente en el log.
- Primera cuenta administradora, login/logout, onboarding y sesiones persistentes.
- PBKDF2-HMAC-SHA256 con salt aleatorio para contraseñas.
- CSRF y rate limit reutilizable.
- Rate limit de login por IP y usuario.
- URLs amigables y frontend con layout/componentes reutilizables.
- Detección de volúmenes Windows y Linux con identidad persistente independiente de letra, nombre o punto de montaje.
- Windows conserva Volume GUID + serial del filesystem; Linux prioriza UUID y conserva PARTUUID/by-id como respaldo de identidad.
- Registro de unidades con nombre, categoría, raíz virtual, solo lectura y timeout de inactividad.
- VFS que unifica todas las unidades registradas sin exponer rutas físicas.
- Montaje bajo demanda y leases para impedir desmontajes durante operaciones activas.
- Auto-desmontaje por inactividad.
- Catálogo persistente separado del estado de autenticación.
- Indexación de archivos con una sola cola para minimizar actividad simultánea sobre HDD y progreso real procesados/total en tiempo real.
- Miniaturas de hasta 320 px y previews de hasta 1600 px para JPEG/PNG/GIF; FFmpeg amplía formatos de imagen y genera portadas multimedia cuando está disponible.
- Galería `/galeria` que usa la caché interna aunque el disco original esté desmontado y oculta medios de unidades físicamente desconectadas.
- Filtro compacto por imágenes/video/audio y orden por fecha de archivo, fecha de incorporación o nombre.
- Visor offline de imagen/video/audio centrado a viewport completo, con navegación ←/→ o A/D, zoom suave W/S y pantalla completa para video.
- Preferencias locales persistentes del reproductor de video: mute, volumen y velocidad.
- Descarga por clic derecho mediante ticket AES-GCM opaco, ligado al usuario y de vida corta; las URLs no revelan ruta, storage ID ni nombre del archivo.
- Scroll infinito por defecto o paginación persistente mediante un componente de listado reutilizable.
- Apertura del original mediante montaje bajo demanda.
- Upload contextual mediante botón/widget dentro de la carpeta actual de `/archivos/ver/...`.
- Registro de unidad con primera indexación automática y reindexación manual visible.
- Explorador `/archivos` basado en el catálogo, usable aunque una unidad esté desmontada o desconectada.
- Upload con destino automático según tipo de archivo, categoría de unidad y espacio libre conocido.
- Políticas por tipo de unidad: documentos, fotos, multimedia o mixto.
- Servidor WebDAV en `/webdav/` con las mismas credenciales.
- TLS directo opcional o despliegue detrás de proxy HTTPS.
- Healthcheck `/salud`.
- Backups diarios de metadatos con retención de siete copias.
- Frontend y todos sus iconos/JS/CSS embebidos funcionan offline, sin CDN.
- Elevación automática: UAC en Windows y `sudo` interactivo en Linux cuando el proceso necesita permisos de volumen y no los tiene.
- Build Linux y Windows/amd64 sin CGO y sin módulos Go externos.

Pendiente deliberado:

- evaluación posterior de SMB; WebDAV sigue siendo el protocolo de archivos remoto estable de esta etapa.

Consulta `tareas/` y `contexto/` para el estado técnico completo.

## Arquitectura

```text
                 navegador / WebDAV
                        |
                  HTTP / HTTPS
                        |
                 Personal Cloud
                        |
              +---------+---------+
              |                   |
             VFS               catálogo
              |             metadata/cache
       Storage Manager             |
              |              SSD interno
       +------+-------+
       |              |
   documentos     multimedia
     HDD/SSD          HDD
```

El catálogo y las miniaturas permanecen en `APP_DATA_DIR`. Los originales permanecen en sus unidades físicas.

Una lectura de original sigue este flujo:

```text
archivo virtual
 -> localizar storage_id
 -> obtener lease
 -> montar unidad si hace falta
 -> stream
 -> liberar lease
 -> esperar idle timeout
 -> desmontar
```

Ver una miniatura o preview no requiere montar el disco original.

## Requisitos

- Go 1.23 o superior para desarrollar/compilar.
- Linux o Windows amd64 para los builds principales actuales.
- Permisos del SO para montar/desmontar si se quiere auto-mount.

No hay dependencias Go externas. `go mod tidy` no necesita resolver módulos adicionales.

## Preparar y probar

```bash
go mod tidy
go test ./...
go vet ./...
```

Scripts de prueba incluidos:

```bat
scripts\test.cmd
```

```bash
./scripts/test.sh
```

La carpeta `scripts/` se reserva para scripts de validación/repetición útil del proyecto; no se incluyen scripts de limpieza residual.

## Ejecutar

```bash
go run ./cmd/server
```

Por defecto escucha en `:8080`.

Primer arranque:

```text
CONFIGURACIÓN INICIAL REQUERIDA url=/setup codigo=ABCD-EFGH-JKLM
```

Abre:

```text
http://127.0.0.1:8080/setup
```

Después de crear el primer administrador, `/setup` queda deshabilitado y se inicia el onboarding.

## Almacenamiento

Abre:

```text
/almacenamiento
```

La pantalla separa unidades registradas de volúmenes detectados.

### Identidad de volúmenes

Linux prioriza UUID de `/dev/disk/by-uuid`; si no existe, usa PARTUUID y además registra un identificador físico de `/dev/disk/by-id` cuando está disponible. Windows usa el Volume GUID como identidad primaria y registra el serial del filesystem como dato auxiliar. No se guarda `/dev/sdb` o `E:` como identidad principal porque esos nombres pueden cambiar.

El servidor tampoco asume que todo USB será reportado como `removable`: HDD/SSD USB pueden presentarse como unidades fijas. Por eso se enumeran volúmenes locales aptos, se excluyen los recursos del sistema y el administrador decide cuáles registrar.

### Categorías

- `Documentos`: documentos, archivos comprimidos y otros archivos generales.
- `Fotos`: imágenes.
- `Multimedia`: imágenes, video y audio.
- `Mixto`: cualquier tipo.

Las políticas también se aplican a PUT WebDAV y uploads web.

### Montaje bajo demanda

Cada operación mantiene un lease. Mientras exista al menos un lease, la unidad no se desmonta.

El timeout se configura por unidad entre 30 segundos y 7 días. El worker de inactividad solo revisa estado en memoria; no recorre el contenido del disco continuamente.

En el cierre limpio del servidor se intenta desmontar las unidades configuradas con auto-desmontaje cuando ya no tienen operaciones activas.

### Windows

La detección usa APIs Win32. Para desmontar, Personal Cloud intenta:

```text
flush
 -> lock volume
 -> dismount volume
 -> retirar punto de montaje
```

Si el volumen está ocupado y no puede bloquearse, la operación falla en lugar de forzar el desmontaje.

Las operaciones de montaje/desmontaje normalmente requieren privilegios elevados. Si el proceso Windows no está elevado, Personal Cloud intenta relanzarse una única vez mediante UAC conservando sus argumentos y directorio de trabajo. La asignación/retiro del punto de montaje usa las APIs Win32 de volumen en vez de depender de `mountvol.exe`.

### Linux

Se leen los montajes desde `/proc/self/mountinfo`. Los mountpoints críticos (`/`, `/boot`, `/boot/efi`, `/usr`, `/var`) y el dispositivo base del root filesystem no pueden registrarse desde la UI.

Montar filesystems requiere privilegios. En una ejecución interactiva sin privilegios, Personal Cloud intenta relanzarse con `sudo` si está disponible. En systemd detecta `CAP_SYS_ADMIN` y no intenta `sudo`; el ejemplo de servicio concede esa capacidad al usuario dedicado y acceso al grupo `disk`.

## Catálogo y Galería

Los metadatos viven en:

```text
data/catalog/snapshot.json
data/catalog/events.jsonl
```

La caché vive en:

```text
data/cache/thumbnails/
data/cache/previews/
```

El catálogo usa un snapshot compacto más un journal append-only. Los eventos se compactan periódicamente y el catálogo activo permanece en memoria para navegación rápida.

Se dejó una deuda técnica explícita: si el catálogo supera aproximadamente 500,000 archivos o el consumo de memoria/tiempo de arranque deja de ser razonable, el paquete `catalog` debe migrarse a un índice disk-backed sin cambiar el VFS.

### Indexación

Al registrar una unidad desde `/almacenamiento`, el servidor inicia la primera indexación automáticamente. La tarjeta registrada muestra **Indexar ahora** para reconciliar cambios hechos fuera de Personal Cloud. El indexador hace una pasada de conteo y otra de procesamiento para mostrar una barra real `procesados / total`; `/api/indexacion` actualiza cada tarjeta en tiempo real mientras está en cola, contando o procesando.

El indexador:

1. obtiene un lease y monta la unidad si hace falta;
2. recorre archivos regulares;
3. ignora symlinks y carpetas de sistema conocidas;
4. actualiza metadata en lotes;
5. genera thumbnails/previews soportados;
6. elimina entradas de archivos que ya no existen;
7. libera el lease;
8. permite que el idle timeout desmonte la unidad.

Solo existe un worker de indexación para no despertar o cargar varios HDD al mismo tiempo.

## Explorador y subida automática

Abre:

```text
/archivos
```

La raíz muestra las unidades como carpetas virtuales. Dentro de cada una, la jerarquía se reconstruye desde el catálogo, por lo que navegar no requiere montar el HDD. Los directorios vacíos todavía no se persisten en el catálogo: aparecen cuando contienen al menos un archivo indexado.

Al abrir un archivo se solicita el original al VFS, que monta solamente su unidad durante la lectura.

La subida ya no ocupa una tarjeta grande en `/almacenamiento`. Al entrar en una carpeta concreta (`/archivos/ver/<raíz>/...`) aparece un botón **Subir aquí** que abre un diálogo compacto y escribe en esa ubicación.

Cuando no se fuerza un destino, el routing usa:

- imágenes: `Fotos` → `Multimedia` → `Mixto`;
- video/audio: `Multimedia` → `Mixto`;
- documentos/archivos/otros: `Documentos` → `Mixto`;
- entre unidades con la misma prioridad se prefiere la que reporta más espacio libre.

Si navegas dentro de una raíz virtual, la subida se dirige explícitamente a esa unidad y sigue aplicando su política de tipos. Después de cada subida se encola la reindexación correspondiente.

### Galería, visor y formatos multimedia

Abre `/galeria`. La ruta histórica `/fotos` redirige permanentemente a la Galería. Los assets del visor son locales/embebidos: no requiere CDN ni Internet.

Controles del visor:

- `←` / `→` o `A` / `D`: medio anterior/siguiente;
- `W` / `S`: zoom suave de la imagen/video visible;
- `Esc`: cerrar;
- video y audio usan los controles HTML5 nativos; el visor añade un botón de pantalla completa para video.
- mute, volumen y velocidad de video se guardan en el navegador y se restauran al cambiar de video o recargar.
- clic derecho sobre un medio permite solicitar una descarga segura mediante ticket opaco.

JPEG, PNG y GIF generan miniatura/preview usando únicamente la biblioteca estándar. Para proteger el MiniPC frente a imágenes que disparen el uso de RAM, una fuente que el decoder nativo identifica con más de 80 megapíxeles se cataloga pero no se decodifica para thumbnail.

FFmpeg es **opcional** y se detecta automáticamente en `PATH`. Cuando existe, Personal Cloud intenta usarlo para:

- generar previews JPEG de imágenes que la biblioteca estándar no decodifica (por ejemplo WebP/HEIC/HEIF/AVIF/RAW si ese build de FFmpeg soporta el codec);
- extraer un frame representativo de video;
- extraer la carátula embebida de audio cuando exista.

Sin FFmpeg el servidor sigue funcionando; únicamente faltarán esas miniaturas.

### Listado reutilizable

Galería y Archivos comparten dos modos:

- **Continuo** (`infinito`): predeterminado; carga automáticamente más elementos al acercarse al final mediante `IntersectionObserver`;
- **Páginas**: navegación anterior/siguiente.

La preferencia se guarda en una cookie local del navegador y el componente se reutiliza en ambas vistas. Los filtros de Galería viajan en la URL para que actualizar, volver atrás o compartir una vista mantenga el criterio elegido.

### Disponibilidad de unidades

La Galería solo consulta medios cuyo `storage_id` pertenece a una unidad registrada y actualmente conectada. La comprobación de presencia usa GUID/UUID y evita consultar capacidad o recorrer contenido, para no despertar el disco únicamente por mantener abierta la Galería. Un volumen desmontado automáticamente por inactividad sigue considerándose disponible: sus miniaturas permanecen visibles y el VFS lo montará al abrir el original. Si el medio físico se desconecta, sus tarjetas desaparecen de la Galería sin borrar el catálogo. Al reconectarlo vuelve a aparecer sin perder metadata.

### Descarga segura

El clic derecho no expone una ruta física ni reutiliza la URL estable del original. El navegador solicita primero `POST /api/descargas`; el servidor devuelve un ticket AES-GCM aleatorio, ligado al usuario autenticado y válido durante un periodo corto. La descarga final vive bajo `/descarga/<ticket>` y usa `Content-Disposition: attachment`, `Cache-Control: no-store` y auditoría. Fuera de loopback, tanto la emisión del ticket como la descarga rechazan HTTP y requieren HTTPS. Los logs redactan el ticket.

Esto protege identificadores/rutas frente a observadores y evita URLs de descarga predecibles. HTTPS sigue siendo obligatorio para confidencialidad de los bytes en tránsito. Si un reverse proxy termina TLS, por definición ese proxy es un extremo de la conexión y puede observar el HTTP descifrado; para impedirlo se necesita TLS passthrough/túnel extremo a extremo, no una cabecera adicional de Personal Cloud.

## WebDAV

Endpoint:

```text
https://tu-dominio/webdav/
```

Usa el mismo usuario y contraseña de Personal Cloud.

Implementado:

- OPTIONS;
- PROPFIND Depth 0/1;
- GET / HEAD;
- PUT;
- DELETE;
- MKCOL;
- MOVE dentro de la misma unidad;
- COPY de archivos;
- LOCK / UNLOCK.

Fuera de loopback WebDAV requiere HTTPS por defecto porque Basic Auth transporta credenciales en cada conexión HTTP.

Para evitar ejecutar PBKDF2 en cada petición válida, el servidor conserva durante cinco minutos una prueba HMAC de la credencial en memoria. No almacena la contraseña y la entrada queda invalidada si cambia el hash persistido del usuario.

Rate limit WebDAV:

- 30 intentos costosos por IP / 15 min;
- 12 por combinación IP + usuario / 15 min;
- las autenticaciones ya validadas en caché no consumen esos intentos.

Las mutaciones WebDAV encolan reconciliación del catálogo. Si una unidad cambia mientras ya se está indexando, se programa una segunda pasada para no perder el cambio.

`COPY` recursivo de directorios no está implementado todavía. Se mantuvo como deuda visible hasta verificar que un cliente real lo requiera.

## Configuración

La aplicación usa variables `APP_*` para configuración de despliegue. No son necesarias para desarrollo local.

| Variable | Predeterminado | Uso |
| --- | --- | --- |
| `APP_ADDR` | `:8080` | Dirección de escucha |
| `APP_DATA_DIR` | `./data` | Estado, catálogo, caché y backups |
| `APP_MOUNT_ROOT` | `/mnt/personalcloud` en Linux | Mountpoints administrados; se ignora en Windows |
| `APP_MAX_UPLOAD_BYTES` | `21474836480` | Máximo por upload, 20 GiB |
| `APP_COOKIE_SECURE` | `false` | Cookie Secure; se fuerza si HTTPS es obligatorio/TLS directo |
| `APP_REQUIRE_HTTPS` | `false` | Redirige HTTP a HTTPS y habilita HSTS |
| `APP_WEBDAV_REQUIRE_HTTPS` | `true` | Rechaza WebDAV remoto sobre HTTP |
| `APP_TLS_CERT_FILE` | vacío | Certificado PEM para TLS directo |
| `APP_TLS_KEY_FILE` | vacío | Clave PEM para TLS directo |
| `APP_TRUSTED_PROXIES` | loopback | CIDRs que pueden aportar forwarded headers |
| `APP_SESSION_TTL` | `168h` | Duración de sesión web |

`APP_TLS_CERT_FILE` y `APP_TLS_KEY_FILE` deben configurarse juntos.

## HTTPS detrás de Caddy

Ejemplo incluido:

```text
deploy/caddy/Caddyfile.example
```

Configura el servicio interno aproximadamente así:

```text
APP_ADDR=127.0.0.1:8080
APP_REQUIRE_HTTPS=true
APP_WEBDAV_REQUIRE_HTTPS=true
APP_TRUSTED_PROXIES=127.0.0.1/32,::1/128
```

Caddy enviará la petición al backend y Personal Cloud solo confiará en forwarded headers procedentes del proxy configurado.

## TLS directo

También puedes hacer que el propio binario escuche TLS:

```text
APP_TLS_CERT_FILE=/ruta/fullchain.pem
APP_TLS_KEY_FILE=/ruta/privkey.pem
```

Si TLS directo está configurado, las cookies pasan automáticamente a `Secure`.

## Backups de metadatos

Una vez que existe administrador, el housekeeping crea como máximo un backup por día en:

```text
data/backups/metadata-YYYYMMDD.zip
```

Retiene las siete copias más recientes.

Incluye:

- `state.json`;
- un snapshot consistente del catálogo;
- manifiesto del backup.

No incluye:

- originales;
- thumbnails/previews, porque son regenerables;
- auditoría completa.

Este backup local protege principalmente ante corrupción/cambios del estado, **no** ante la pérdida física del SSD interno. Un backup secundario/off-site es una fase distinta.

## Despliegue Linux con systemd

Archivos:

```text
deploy/linux/personalcloud.service
deploy/linux/personalcloud.env.example
deploy/linux/install-systemd.sh
```

Compila primero:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o personalcloud ./cmd/server
```

En una distribución systemd con `useradd`, `usermod` y grupo `disk`:

```bash
sudo ./deploy/linux/install-systemd.sh ./personalcloud
```

Después revisa:

```text
/etc/personalcloud/personalcloud.env
```

Comandos útiles:

```bash
sudo systemctl status personalcloud
sudo journalctl -u personalcloud -f
sudo systemctl restart personalcloud
```

La capacidad `CAP_SYS_ADMIN` necesaria para montar es amplia. El servicio compensa con usuario dedicado y sandbox de systemd, pero una futura separación de helper privilegiado puede ser conveniente si el servidor aumenta su superficie de exposición.

## Inicio automático en Windows

Archivos:

```text
deploy/windows/install-startup-task.ps1
deploy/windows/uninstall-startup-task.ps1
```

Compila:

```bat
go build -trimpath -ldflags="-s -w" -o personalcloud.exe ./cmd/server
```

Abre PowerShell como administrador:

```powershell
.\deploy\windows\install-startup-task.ps1 -ExePath .\personalcloud.exe
```

Se crea una tarea **Personal Cloud** al inicio, ejecutada con el usuario actual mediante S4U y nivel elevado. El directorio de trabajo predeterminado es:

```text
C:\ProgramData\PersonalCloud
```

Para retirar únicamente la tarea:

```powershell
.\deploy\windows\uninstall-startup-task.ps1
```

El desinstalador no borra datos.

## Compilar

Linux estático:

```bash
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/personalcloud ./cmd/server
```

Windows:

```bat
if not exist bin mkdir bin
go build -trimpath -ldflags="-s -w" -o bin\personalcloud.exe ./cmd/server
```

Cross-build Windows desde Linux:

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/personalcloud.exe ./cmd/server
```

## Seguridad relevante

- `/setup` solo funciona mientras no exista administrador.
- El setup code vive únicamente en memoria.
- Contraseñas y tokens nunca se escriben en logs.
- Sesiones guardan solamente el hash SHA-256 del token.
- CSRF protege formularios web mutables.
- El VFS rechaza traversal y symlinks que escapen de una raíz registrada.
- Los uploads tienen límite y se escriben a temporales antes de reemplazar el archivo final.
- Un volumen `read_only` no admite operaciones mutables desde Web ni WebDAV.
- `X-Forwarded-For`/`X-Forwarded-Proto` solo se aceptan desde CIDRs confiables.
- WebDAV remoto requiere HTTPS por defecto.
- La aplicación no fuerza un dismount Windows si no consigue un lock exclusivo.

## Prueba manual recomendada de R7

### Núcleo

1. Conserva tu `data/` actual y arranca R7; el estado existente se mantiene compatible.
2. Confirma que tu cuenta y onboarding siguen presentes.
3. Comprueba `/inicio`, `/almacenamiento`, `/galeria`, `/archivos` y `/salud`.

### Unidad física

1. Conecta un USB/HDD/SSD que **no** sea del sistema.
2. Abre `/almacenamiento` y verifica que aparezca como detectado.
3. Regístralo, por ejemplo como `Fotos`, raíz `Fotos` y timeout 60 segundos.
4. Observa la barra de progreso hasta que llegue al 100 %.
5. Comprueba que imágenes/videos/audio aparecen en `/galeria`.
6. Espera el timeout y verifica que la unidad pase a desmontada.
7. Recarga `/galeria`: las miniaturas deben seguir cargando desde el almacenamiento interno.
8. Abre un original: la unidad debe montarse bajo demanda.
9. Cierra la transferencia y comprueba que vuelve a desmontarse después del timeout.

### Upload

1. Entra a `/archivos/ver/<raíz>` o una subcarpeta.
2. Pulsa **Subir aquí** y verifica que aparezca el diálogo compacto.
3. Sube un archivo compatible con la política de la unidad.
4. Verifica que un tipo incompatible sea rechazado.
5. Comprueba que la reindexación se encola automáticamente.

### WebDAV

Conecta un cliente a:

```text
http://127.0.0.1:8080/webdav/
```

para una prueba local. Para cualquier acceso remoto usa HTTPS.

Comprueba crear carpeta, subir archivo, leerlo, renombrarlo y borrarlo.

## Contexto y tareas

Las decisiones persistentes están en `contexto/` y el estado del roadmap en `tareas/`. Solo se marca una tarea como completada cuando su código y validación mínima existen; las limitaciones detectadas se convierten en tareas futuras en lugar de ocultarse.
