# internal/servidor — Aplicación web

Handlers HTTP del panel de gestión: dashboard, formularios y API de estado.

## Rutas

| Método | Ruta | Handler | Acción |
|---|---|---|---|
| GET | `/` | `dashboard` | Dibuja `index.html` con las instancias |
| POST | `/instancias` | `crearInstancia` | Botón **Aceptar** (campo `nombre`) |
| POST | `/publicar` | `publicarContenido` | Botón **Publicar** (`nombre` + archivo `archivo`) |
| POST | `/instancias/{nombre}/eliminar` | `eliminarInstancia` | Botón **Eliminar** |
| POST | `/instancias/{nombre}/apagar` | `apagarInstancia` | Botón **Apagar** (solo en `lista`/`publicada`) |
| POST | `/instancias/{nombre}/encender` | `encenderInstancia` | Botón **Encender** (solo en `apagada`) |
| GET | `/api/instancias` | `apiInstancias` | Listado en JSON (lo consulta `app.js`) |
| GET | `/static/...` | `FileServerFS` | CSS y JS incrustados |

Usa el enrutador de Go 1.22+ (`"POST /instancias/{nombre}/eliminar"` y `r.PathValue("nombre")`). No requiere librerías externas.

## Patrón POST → Redirect → GET

Cada formulario responde con un **303** hacia `/?tipo=ok|error&msg=...`. El dashboard muestra ese mensaje, y recargar la página (F5) no repite la acción.

## Subida del `.zip` (`subida.go`)

1. `http.MaxBytesReader` corta la petición si supera `tamano_maximo_mb`.
2. La extensión debe ser `.zip`.
3. Los 4 primeros bytes deben ser la firma ZIP `PK\x03\x04`. Así se rechaza un archivo cualquiera renombrado.
4. Se guarda en un temporal (`/tmp/httppaas-*.zip`), que el orquestador borra al terminar.

## Actualización automática

La página carga `app.js`, que cada 4 s consulta `/api/instancias`. Si cambió algún estado, recarga, **excepto** si el usuario está escribiendo un nombre o eligió un archivo, para no borrar lo que estaba haciendo.

## Seguridad de la plantilla

`html/template` escapa automáticamente todo el texto (nombres, mensajes, errores). Un valor malicioso no puede inyectar HTML ni JavaScript.

## Registro

El middleware `registrarPeticiones` escribe en la terminal `[web] MÉTODO /ruta (duración)`. Omite `/api/instancias` y `/static/` para no llenar la terminal.
