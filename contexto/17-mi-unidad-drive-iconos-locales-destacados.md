# 17. Mi unidad tipo Drive, iconos locales y Destacados

Fecha: 2026-08-15

## Objetivo

Ajustar `Mi unidad` para que se comporte visualmente como Google Drive: mostrar primero las carpetas del primer nivel y, después, los archivos. La raíz deja de representar las unidades físicas como si fueran carpetas del usuario.

## Namespace virtual de Mi unidad

- `/archivos` combina el primer nivel de todas las unidades registradas que están **disponibles**.
- Las carpetas se ordenan antes que los archivos.
- Cada carpeta conserva internamente su `VirtualRoot`, por lo que al abrirla se navega hacia la unidad física correcta sin exponerla como una carpeta artificial.
- Las unidades desconectadas no aportan elementos al namespace raíz. Siguen administrándose desde Almacenamiento y su catálogo puede seguir consultándose entrando explícitamente a su ruta cuando corresponda.
- La raíz conserva filtros, selector lista/cuadrícula, paginación y scroll infinito.
- Cuando existen unidades registradas pero ninguna aporta contenido disponible se muestra un estado vacío específico en lugar de tarjetas de discos.

## Detección de tipos e iconos

Se centralizó la detección por extensión en `internal/storage/policy.go` y se amplió para imágenes, audio, video, documentos, Office, comprimidos, APK/AAB, ejecutables, bases de datos, código, ebooks, fuentes y certificados.

Para los tipos más reconocibles se integraron SVG de Material Icon Theme 5.37.0 obtenidos desde jsDelivr durante el desarrollo y guardados en `web/static/icons/`. No se consulta ningún CDN en tiempo de ejecución. Se incluye la licencia MIT del proyecto original.

Iconos locales específicos incluidos, entre otros:

- Android/APK/AAB
- PDF
- Markdown
- Word
- Excel/hojas de cálculo
- PowerPoint
- imagen, audio y video
- bases de datos
- archivos comprimidos
- ejecutables
- documentos genéricos

Los tipos sin recurso específico conservan el icono local de hoja + etiqueta de extensión, de modo que nunca dependen de red.

## Destacados

- El visor multimedia permite agregar o quitar el elemento actual de Destacados.
- Atajo `F` dentro del visor para alternar Destacados.
- La selección múltiple incorpora `Agregar a Destacados` / `Quitar de Destacados`.
- La operación múltiple actualiza todos los IDs en una sola mutación persistente y registra auditoría.
- El estado se sincroniza entre tarjeta, visor, menú contextual y selección.

## Página principal

La página principal construye Carpetas sugeridas y Archivos sugeridos únicamente con unidades registradas que estén online. Un archivo perteneciente a una unidad desconectada ya no se propone en Inicio.

## Compatibilidad

No se agregó npm, framework frontend ni dependencia remota de ejecución. Los assets continúan embebidos en el binario Go.
