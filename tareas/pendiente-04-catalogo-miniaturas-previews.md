# Tarea 04 — Catálogo, miniaturas y previews

Indexar archivos de las unidades, conservar metadatos en un índice persistente adecuado a la escala real y generar miniaturas/previews en el almacenamiento interno para navegar fotos sin montar el disco del original. Al abrir el original, resolver `storage_id`, montar y hacer streaming.

SQLite sigue siendo un candidato para el catálogo, pero la dependencia se incorporará aquí solo si las pruebas de escala y portabilidad la justifican. El estado pequeño de autenticación/sesiones permanece separado del catálogo.
