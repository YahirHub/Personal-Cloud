# 20. Eliminación contextual, HTML aislado y almacenamiento unificado

Fecha: 2026-08-15

## Objetivo

Completar la experiencia tipo Drive con acciones destructivas coherentes desde cualquier superficie, aislar de forma estricta la vista de archivos HTML y presentar las unidades conectadas como una sola capacidad lógica sin perder el desglose físico.

## Eliminación y clic derecho

- El diálogo nativo `window.confirm` deja de ser la experiencia principal de eliminación. El layout monta un diálogo reutilizable con título, detalle de irreversibilidad y botón destructivo explícito.
- El menú contextual de archivo sigue siendo único y se reutiliza desde tarjetas, filas y el visor multimedia.
- En modo de selección, el clic derecho sobre un elemento abre un menú para **toda la selección** con Destacados, ZIP, Mover y Eliminar.
- El visor multimedia incluye papelera propia. El visor de documentos incluye la misma acción y ambos delegan en `/api/elementos/eliminar` mediante la función global reutilizable, por lo que no se duplica lógica destructiva.
- La eliminación exige confirmación antes de tocar los originales y después limpia catálogo/cachés mediante el flujo existente.
- La pulsación larga táctil conserva la equivalencia con el clic derecho.

## Visor HTML aislado

El HTML de usuario ya no se carga como un documento HTML de la misma aplicación para la vista normal.

1. El contenido se obtiene como texto mediante el endpoint autenticado existente.
2. `DOMParser` lo procesa en un documento separado, nunca se inserta con `innerHTML` en el DOM de Nube.
3. Se retiran `script`, `iframe`, `frame`, `frameset`, `object`, `embed`, `applet` y `base`, además de atributos `on*` y meta refresh/CSP aportadas por el archivo.
4. Se inyecta una CSP de vista previa con `default-src 'none'`, sin scripts, red, formularios, frames u objetos.
5. La vista se asigna a `iframe.srcdoc` manteniendo `sandbox=""`. Al no conceder `allow-same-origin`, el documento obtiene un origen opaco y no comparte privilegios, DOM ni CSS con Personal Cloud.
6. Solo se mantienen estilos inline y recursos `data:`/`blob:` permitidos por la CSP para una vista estática útil.

El endpoint HTML histórico se conserva endurecido por compatibilidad, pero el visor principal usa `srcdoc` aislado.

## Almacenamiento unificado

La capacidad lógica se calcula en tiempo real con `storage.Manager.Views` y únicamente contabiliza vistas que cumplan:

- unidad registrada;
- unidad online/conectada;
- capacidad conocida mayor que cero.

Para cada unidad:

- `usado = capacidad - libre`;
- `libre` se limita como máximo a `capacidad` para tolerar metadatos anómalos;
- se calcula un porcentaje individual.

El resumen global suma capacidad, usado y libre de todas esas unidades y muestra el número de medios online. Las unidades desconectadas no aportan capacidad fantasma y los volúmenes detectados pero no registrados aún no forman parte de la nube lógica.

## Interfaz de almacenamiento

- La barra lateral enlaza a `/almacenamiento` y muestra `usado / total`, espacio libre, número de unidades y una barra compacta.
- `/almacenamiento` comienza con filtros reales de **Tipo**, **Modificado** y **Fuente**, seguidos por un resumen grande tipo Drive del total agregado. Los filtros afectan al ranking de archivos y no falsean las métricas físicas de capacidad.
- Sigue un desglose de cada unidad conectada con usado, libre, total y porcentaje.
- Se muestran hasta 40 archivos online ordenados por tamaño para localizar rápidamente qué consume espacio.
- El bloque de administración anterior se conserva al final para montaje, indexación, integridad y configuración; no se sustituyen funciones reales por controles decorativos.

## Seguridad y consistencia

- Las acciones destructivas siguen usando sesión, CSRF, rate limits y VFS.
- La vista HTML no obtiene acceso a `window.parent`, cookies, sesión ni estilos de la aplicación debido al sandbox sin `allow-same-origin` y a su CSP.
- El listado de archivos grandes solo incluye medios online, evitando enlaces a originales imposibles de abrir.
- El cálculo de almacenamiento no depende del catálogo: refleja el uso real del filesystem, incluyendo archivos externos a Personal Cloud presentes en la unidad.
