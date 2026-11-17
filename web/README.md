# web — Interfaz visual

HTML, CSS y JavaScript del panel. Se **incrustan en el binario** con `go:embed` (`web.go`), así que el ejecutable funciona solo, sin copiar esta carpeta.

```
web/
├── web.go                 Expone Plantillas y Estaticos (embed.FS)
├── templates/index.html   Página única: cabecera, formularios, tabla
└── static/
    ├── estilos.css        Tema oscuro morado (misma línea del Servidor de Imágenes)
    └── app.js             Confirmación al eliminar + refresco automático
```

## Secciones de la página

| Sección | Contenido |
|---|---|
| Cabecera | Título, dominio (`cloud.local`) y hostname del PC |
| Mensaje | Resultado de la última acción (verde = ok, rosa = error) |
| **1 · Nueva instancia** | *Nombre del host* + sufijo `.cloud.local` → **Aceptar** |
| **2 · Publicar contenido** | Instancia (solo las `lista`/`publicada`) + *Contenido web (comprimido)* → **Publicar** |
| **Instancias disponibles** | Enlace (nueva pestaña), IP/máscara, host/VM, estado con detalle, fecha, **Eliminar** |

## Datos que recibe la plantilla (`servidor.DatosPagina`)

`Titulo`, `Zona`, `Equipo`, `Instancias`, `Publicables`, `HayProcesos`, `Mensaje`, `TipoMensaje`, `MaximoMB`. Función extra: `{{fecha .CreadaEn}}` → `2026-11-16 17:02:03`.

## Diseño

- Colores definidos como variables en `:root` (`--page`, `--surface`, `--accent`...).
- Colores de estado: verde (`lista`, `publicada`), azul con indicador giratorio (en proceso), rosa (`error`).
- **Móvil (≤ 760 px):** los formularios se apilan y la tabla se convierte en tarjetas con etiquetas.
- Respeta `prefers-reduced-motion` (sin animaciones si el sistema lo pide).

## Editar la interfaz

Los archivos se incrustan **al compilar**: tras cambiar HTML, CSS o JS hay que volver a ejecutar `go build` o `go run`.
