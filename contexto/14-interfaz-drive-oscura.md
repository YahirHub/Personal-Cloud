# Fecha
2026-08-15

# Objetivo
Rehacer la interfaz web para que la experiencia resulte inmediatamente familiar a usuarios acostumbrados a Google Drive, conservando modo oscuro fijo, funcionamiento 100% offline y únicamente las capacidades que Personal Cloud implementa realmente.

# Decisiones tomadas
- La aplicación autenticada usa una composición equivalente a Drive: barra superior fija, marca a la izquierda, búsqueda central, cuenta/configuración a la derecha, navegación lateral y superficie principal redondeada.
- El tema de la aplicación autenticada queda fijado a oscuro; no depende de `prefers-color-scheme`.
- Se conserva una identidad propia (`Nube`) y recursos SVG locales; no se añade CDN, fuente remota ni runtime frontend.
- `Página principal` deja de ser un dashboard de métricas y pasa a mostrar `Carpetas sugeridas` y `Archivos sugeridos`, usando datos reales del catálogo.
- Las carpetas sugeridas se derivan de raíces virtuales registradas; los archivos sugeridos son los elementos más recientes del catálogo y reutilizan miniaturas locales cuando existen.
- `Mi unidad` reemplaza la presentación visual anterior de `Archivos` y usa breadcrumbs, tarjetas de carpeta y vista cuadrícula/lista similar a Drive.
- La preferencia cuadrícula/lista se conserva en `localStorage` y no necesita persistencia de servidor.
- El botón global `Nuevo` abre la subida en `Mi unidad`; desde otras páginas navega a `/archivos?nuevo=1` y abre el diálogo automáticamente.
- Se añadió búsqueda global real por nombre/ruta desde la barra superior. Se muestran como máximo 300 coincidencias, ordenadas por modificación reciente.
- Los menús de tres puntos de tarjetas reutilizan el menú seguro existente de descargar/mover/eliminar; no se muestran acciones falsas.
- Galería, Almacenamiento, Configuración, diálogos, menús y formularios heredan el mismo lenguaje Material oscuro para que el cambio sea global.
- En móvil la barra lateral se convierte en navegación inferior compacta para no sacrificar el área principal.

# Arquitectura actual

```text
base.html
  -> topbar.html (marca + búsqueda + cuenta)
  -> sidebar.html (Nuevo + navegación)
  -> página
       -> Inicio: sugeridos reales
       -> Mi unidad: raíces / carpetas / archivos + grid/list
       -> Galería / Almacenamiento / Configuración
```

La búsqueda sigue el flujo:

```text
GET /archivos?q=texto
 -> catálogo local en memoria
 -> coincidencia sobre nombre + raíz virtual + ruta relativa
 -> estado online/offline desde Storage Manager
 -> vista de resultados sin montar unidades
```

# Librerías usadas
Ninguna nueva. Go estándar, templates HTML, CSS y JavaScript local embebido.

# Archivos importantes modificados
- `internal/app/app.go`
- `internal/app/files_handlers.go`
- `internal/app/storage_handlers.go`
- `internal/app/viewmodels.go`
- `web/layouts/base.html`
- `web/components/topbar.html`
- `web/components/sidebar.html`
- `web/components/icons.html`
- `web/pages/dashboard.html`
- `web/pages/files.html`
- `web/pages/photos.html`
- `web/static/app.css`
- `web/static/app.js`
- `internal/webui/renderer_test.go`

# Problemas encontrados
- La página principal anterior era un dashboard técnico y no se parecía al flujo que los estudiantes ya reconocen.
- El explorador solo tenía una vista de filas, mientras Drive alterna cuadrícula/lista y prioriza tarjetas con previews.
- No existía búsqueda global desde la interfaz.
- El botón de subida solo aparecía dentro de carpetas concretas y no podía funcionar como el botón global `Nuevo`.

# Soluciones implementadas
- Shell visual completo tipo Drive en oscuro.
- Inicio con carpetas/archivos sugeridos reales.
- `Mi unidad` con grid/list persistente y miniaturas del catálogo.
- Búsqueda global funcional sin despertar discos.
- `Nuevo` global con destino automático cuando se usa desde la raíz.
- Tres puntos conectados a acciones existentes.
- Adaptación responsive y tema oscuro persistente de controles/selects.

# Pendientes
- Validar visualmente en los navegadores y resoluciones reales de los equipos de la escuela, especialmente escalado de Windows 125%/150%.
- Si se requiere replicar también comportamiento específico de Drive (arrastrar y soltar, renombrado inline, favoritos, compartidos, etc.), debe implementarse como funcionalidad real antes de mostrar sus controles.

# Próximos pasos
Probar con un catálogo real que contenga documentos, imágenes, video y archivos comprimidos; revisar la densidad de tarjetas en las pantallas de los estudiantes y ajustar únicamente espaciado/tamaños sin romper la estructura familiar.
