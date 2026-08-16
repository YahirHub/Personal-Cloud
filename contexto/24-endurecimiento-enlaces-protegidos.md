# 24. Endurecimiento de enlaces públicos protegidos

Fecha: 2026-08-15

## Objetivo

Evitar que la autorización temporal emitida después de introducir la contraseña de un enlace público pueda convertirse accidentalmente en un segundo enlace compartible. En particular, copiar una URL interna como `/s/<token>/contenido?access=...` y abrirla en otro perfil o ventana de incógnito no debe saltarse la contraseña.

## Modelo corregido

### Navegación normal

`GET /s/<token>` ya no acepta `access` como mecanismo para desbloquear la página. Después de validar la contraseña:

- se emite una cookie HttpOnly ligada a `share_id + token + password_hash`;
- la cookie es **de sesión**, no una autorización persistente de 12 horas;
- el navegador vuelve mediante `303` a la URL limpia `/s/<token>`;
- ningún grant queda en barra de direcciones, historial o enlace copiable.

Si llega una URL antigua `/s/<token>?access=...`, se canonicaliza a la URL limpia antes de renderizar.

Cambiar la contraseña o renovar el token sigue invalidando automáticamente la firma de la cookie.

### Embed protegido

Un iframe de otro origen puede tener las cookies de terceros bloqueadas. Para conservar ese caso sin volver a colocar una credencial en la URL del iframe, el POST correcto de contraseña renderiza directamente el embed autorizado.

Sólo los subrecursos internos de esa respuesta reciben un grant HMAC `v2`:

- vida máxima de 2 horas;
- ligado al `share_id`, token actual y hash de contraseña;
- no autoriza `GET /s/<token>` ni `/embed`;
- sólo se acepta cuando el navegador envía un `Referer` same-origin desde `/s/<token>` o `/s/<token>/embed` del **mismo token**;
- si existe `Sec-Fetch-Site`, debe ser `same-origin`;
- una URL `.../contenido?access=...` pegada directamente en otra ventana carece de ese contexto y responde `401`.

Los iframes internos de HTML/PDF/texto usan `referrerpolicy="same-origin"` para que el grant de subrecurso pueda validarse sin enviar referencias a terceros.

### Caché

Todo contenido protegido conserva `Cache-Control: private, no-store`. Esto se amplió también a variantes de video transcodificadas; los enlaces sin contraseña pueden mantener la caché privada de variantes.

## Compatibilidad y revocación

El formato anterior `exp.firma` deja de aceptarse; los grants nuevos son `v2.exp.firma`. Al desplegar una nueva versión el secreto HMAC de proceso ya se rota durante el arranque, pero el cambio de formato evita depender de ese detalle.

Renovar el enlace o cambiar la contraseña invalida tanto cookies como grants. Cerrar la sesión del navegador elimina el desbloqueo normal al tratarse de una cookie de sesión.

## Pruebas de regresión

- un grant válido sin `Referer` no autoriza el recurso;
- el mismo grant sí autoriza el subrecurso iniciado por el embed exacto;
- un `Referer` de otro host o de otro token se rechaza;
- un grant de subrecurso no autoriza la página pública;
- la cookie de desbloqueo es HttpOnly y de sesión;
- cambiar contraseña o token invalida el grant firmado.
