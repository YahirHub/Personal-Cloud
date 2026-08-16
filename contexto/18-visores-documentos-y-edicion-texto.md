# 18. Visores de documentos y edición de texto

Fecha: 2026-08-15

## Objetivo

Abrir Markdown, HTML, texto plano y PDF directamente desde Personal Cloud sin depender de servicios externos, manteniendo las acciones habituales de Drive y permitiendo editar de forma segura los formatos basados en texto.

## Tipos soportados

- Markdown: `.md`, `.markdown`, `.mdown`, `.mkd`.
- HTML: `.html`, `.htm`, `.xhtml`.
- Texto: `.txt`, `.text`, `.log`, `.rst`.
- PDF: `.pdf`.

El backend publica el tipo de visor y si el archivo es editable en los modelos de Mi unidad/Inicio y en `/api/archivo/{id}/info`, por lo que el frontend no adivina el comportamiento a partir del MIME del navegador.

## Visor unificado

Se añadió un `dialog` global estilo Drive disponible desde Mi unidad, Página principal, Recientes, Destacados y los menús `⋯`.

Todos los visores incluyen:

- nombre/tipo/tamaño;
- agregar o quitar de Destacados;
- descarga mediante el ticket seguro ya existente;
- cierre con `Esc`;
- estado de carga y errores de unidad desconectada.

### Markdown

Markdown se interpreta enteramente con JavaScript local y nodos DOM seguros, sin `innerHTML` para el contenido del usuario y sin librerías/CDN. Se contemplan encabezados, énfasis, tachado, código inline y en bloque, enlaces HTTP/HTTPS/mailto, citas, listas, tareas y separadores. El HTML crudo incrustado en Markdown se trata como texto y no se ejecuta.

### HTML

El HTML se sirve en un endpoint específico y se muestra en un `iframe` sandbox sin permisos. Una CSP adicional bloquea scripts, conexiones de red, formularios, frames y plugins; sólo se permiten estilos inline y recursos `data:`/`blob:` necesarios para una previsualización estática. Existe además la pestaña `Código` para ver el fuente.

### Texto

Los archivos de texto se muestran con tipografía monoespaciada conservando espacios y saltos de línea.

### PDF

Los PDF se sirven `inline` mediante `http.ServeContent`, de modo que el visor nativo del navegador puede usar peticiones Range y no es necesario cargar el documento completo en RAM. El endpoint sólo puede ser embebido por el mismo origen.

## Edición

Markdown, HTML y texto plano se pueden editar en el propio visor.

- límite: 8 MiB por archivo;
- sólo UTF-8 y sin bytes NUL;
- `Ctrl+S` / `Cmd+S` guarda;
- `Tab` inserta dos espacios;
- se avisa antes de cerrar o cancelar con cambios sin guardar;
- el POST exige sesión + CSRF y está limitado por rate limit;
- el guardado usa `VFS.WriteAtomic`, por lo que no se escribe directamente sobre rutas físicas desde el handler;
- antes de guardar se compara una versión optimista derivada de `mtime + size`; si el archivo cambió desde que se abrió, se responde `409 Conflict` para evitar pisar silenciosamente cambios externos;
- tras guardar se actualizan tamaño, fecha/MIME y catálogo, y queda auditoría `file_text_edit`.

El catálogo sigue respetando las reglas de reconexión del VFS: si la unidad vuelve a estar disponible, el visor abre/guarda el original; si está realmente offline se informa sin alterar el archivo.

## Sin dependencias remotas

No se añadió npm, framework frontend, parser Markdown externo ni visor PDF remoto. HTML/CSS/JS siguen embebidos en el binario Go y las descargas reutilizan el mecanismo local de tickets opacos.
