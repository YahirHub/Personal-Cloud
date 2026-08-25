# URL pública canónica configurable

## Problema

Los enlaces públicos se generaban con el esquema y `Host` de la petición entrante. Detrás de un túnel o proxy, eso puede producir URLs internas como `https://127.0.0.1:8736/s/...`.

## Solución

La configuración admite `APP_URL`, equivalente al concepto de URL pública base de Laravel. Cuando está definida, los enlaces absolutos de recursos compartidos usan esta URL y dejan de depender del `Host` interno del origen.

Ejemplo:

```env
APP_URL=https://ncloud.admvo.org
```

La variable debe ser una URL absoluta `http` o `https`, sin credenciales, query ni fragmento. Se eliminan barras finales para evitar URLs duplicadas.

Si `APP_URL` no está definida, se conserva el comportamiento anterior basado en la petición actual como fallback.
