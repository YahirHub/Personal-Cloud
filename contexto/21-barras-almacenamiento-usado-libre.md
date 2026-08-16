# 21. Barras de almacenamiento usado y libre

Fecha: 2026-08-15

## Problema

Las barras de capacidad de la barra lateral, el resumen global y cada unidad física se renderizaban con un `span` cuyo ancho se definía mediante `style=\"width:...\"`. La política CSP global usa `style-src 'self'`, por lo que el navegador bloqueaba esos estilos inline. Al quedar el `span` sin ancho explícito y ser un bloque, ocupaba el 100% de la pista y visualmente parecía que todo el almacenamiento estaba usado aunque los números de usado/libre fueran correctos.

## Solución

- Se sustituyeron las barras basadas en estilos inline por elementos HTML nativos `<progress>`.
- El valor de cada barra usa el porcentaje calculado por el backend (`PercentUsed`) y `max=100`.
- La parte usada se representa en azul y el resto de la pista representa de forma visible el espacio libre en gris.
- Se aplicó el mismo componente a la barra lateral, al total global y a cada unidad conectada.
- Se añadieron estilos compatibles con Chromium/WebKit y Firefox mediante `::-webkit-progress-*` y `::-moz-progress-bar`.
- Se corrigió el helper de plantilla `percent`: ahora calcula el porcentaje de la porción recibida en lugar de invertirla. Esto corrige también el porcentaje etiquetado como **libre** en la administración avanzada de cada unidad.
- Se mantuvo la CSP estricta; no se añadió `unsafe-inline`.
- Las etiquetas accesibles indican porcentaje usado y libre.

## Resultado

La proporción visual ahora coincide con las cifras físicas. Por ejemplo, 50.5 GiB usados de 115.3 GiB muestran aproximadamente 44% azul y 56% gris, y cada unidad representa su propia relación usado/libre.
