# 22. Destino manual de subida y enlaces públicos

Fecha: 2026-08-15

## Objetivo

Completar dos flujos tipo Google Drive:

1. permitir que **Nuevo → Subir archivo** use el enrutamiento automático existente o que el usuario fuerce la unidad y carpeta exactas;
2. permitir compartir un archivo mediante un enlace público revocable, opcionalmente protegido con contraseña, con administración centralizada y soporte de embed.

## Subida con destino explícito

El diálogo de subida conserva **Automático** como opción predeterminada. Al pulsar **Elegir ubicación** se puede:

- seleccionar una unidad registrada, conectada y escribible;
- navegar sus carpetas mediante `/api/carpetas`;
- subir de nivel;
- escribir una ruta relativa manual;
- crear de forma segura la jerarquía si todavía no existe.

El backend recibe `destination_root` antes del stream del archivo, valida que la unidad exista, esté online, no sea de solo lectura y admita el tipo de archivo por su categoría. Si no se envía `destination_root`, se conserva exactamente el comportamiento anterior: carpeta actual cuando se navega dentro de una unidad, o selección automática por categoría/espacio libre desde la raíz lógica.

Las auditorías distinguen `file_upload_manual` y `file_upload_auto`.

## Enlaces públicos

El estado persistente sube a versión 5 e incorpora `PublicShare`. Cada enlace guarda:

- propietario;
- `file_id` lógico;
- token aleatorio de alta entropía;
- hash opcional de contraseña;
- fechas de creación/actualización/último acceso;
- contador de accesos.

El token es una credencial portadora y nunca se registra en claro en el logger HTTP: las rutas `/s/<token>/...` se redactan como `/s/{token}/...`.

### Administración

La vista `/compartidos` permite:

- copiar la URL pública;
- copiar la URL embed;
- cambiar entre acceso por enlace o enlace + contraseña;
- conservar o cambiar la contraseña;
- renovar el token y dejar inválida la URL anterior;
- revocar un enlace;
- eliminar todos los enlaces propios;
- para administradores, ver/gestionar enlaces de todos los usuarios y revocarlos globalmente.

Las mutaciones usan sesión, CSRF, rate limit y auditoría. El administrador opera sobre el `share_id` real, por lo que editar un enlace creado por otro usuario no crea accidentalmente un segundo enlace.

### Contraseñas y embeds

Las contraseñas públicas se almacenan con el mismo PBKDF2-HMAC-SHA256 robusto usado por cuentas, con salt aleatorio y validación específica de longitud.

El desbloqueo de un enlace público no modifica estado autenticado del usuario, así que no depende de la cookie CSRF principal. Esto permite que un formulario de contraseña funcione dentro de un iframe aun cuando el navegador bloquee cookies de terceros. Tras validar la contraseña se emite:

- una cookie HTTP-only por `share_id` para navegación normal;
- un ticket HMAC temporal ligado a `share_id + token + password_hash`, usado por el embed.

Cambiar la contraseña **o renovar el enlace** invalida automáticamente cookies/tickets anteriores. El rate limit protege intentos de contraseña.

### Visores públicos

`/s/<token>` muestra una página pública y `/s/<token>/embed` la versión embebible.

- imagen: canvas de imagen;
- video: reproductor propio del proyecto;
- audio: reproductor HTML de audio;
- PDF/texto/Markdown: visor aislado;
- HTML: iframe sandbox con CSP estricta;
- formatos no previsualizables: descarga.

El video público soporta el mismo concepto de calidad adaptativa: Original y variantes 360p/480p/720p/1080p cuando FFmpeg + libx264 estén disponibles. Auto mide una muestra real, considera el tamaño del reproductor y prepara la variante bajo demanda. El cambio de resolución usa un video de staging para conservar tiempo/estado y sustituir la fuente cuando ya está lista. La preparación pública está rate-limited por enlace e IP y exige un header propio de `fetch`, evitando que un formulario cross-site pueda disparar transcoding a ciegas.

Los endpoints de contenido usan `http.ServeContent`, por lo que conservan Range/seek. El HTML compartido no se mezcla con el DOM de Nube y scripts/red/formularios quedan bloqueados.

## Ciclo de vida

- Renombrar o mover un archivo migra su `file_id` en los enlaces y conserva la URL pública.
- Eliminar un archivo revoca sus enlaces.
- Si una unidad está desconectada, el enlace permanece y muestra temporalmente no disponible; vuelve a servir el mismo archivo al reconectar.
- Renovar un enlace cambia el token sin cambiar el archivo ni la contraseña.

## Seguridad

- tokens generados con `crypto/rand`;
- contraseñas PBKDF2 y nunca recuperables;
- CSP estricta y HTML sandbox;
- rate limit para gestión, contraseñas y transcoding público;
- no se exponen rutas físicas ni `storage_id`;
- el embed es el único documento público que permite `frame-ancestors *`; el shell privado mantiene `DENY`;
- scripts del panel autenticado no se cargan en páginas públicas.
