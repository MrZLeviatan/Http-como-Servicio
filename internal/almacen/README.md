# internal/almacen — Persistencia de instancias

Guarda el listado de instancias en un archivo JSON (`data/instancias.json`) para que el dashboard no se pierda al reiniciar la aplicación.

## API

| Método | Descripción |
|---|---|
| `Abrir(ruta)` | Carga el archivo (o empieza vacío si no existe) |
| `Listar()` | Copias de todas las instancias, de la más antigua a la más nueva |
| `Obtener(nombre)` | Copia de una instancia o `ErrNoExiste` |
| `Agregar(inst)` | Registra una nueva; falla si el nombre ya existe |
| `Actualizar(nombre, func)` | Modifica una instancia de forma **atómica** (ver abajo) |
| `Eliminar(nombre)` | La quita del registro |

## Decisiones de diseño

**Concurrencia.** La web y las tareas en segundo plano acceden a la vez. Un `sync.Mutex` asegura que solo una goroutine toque el mapa en cada momento. Se devuelven **copias**, nunca punteros internos.

**Actualizar con verificación.** `Actualizar` ejecuta la función de cambio *con el candado tomado*. Así se puede comprobar el estado y cambiarlo en un solo paso, sin que otra petición se cuele en medio:

```go
alm.Actualizar("paisajes", func(i *modelo.Instancia) error {
    if !i.PuedePublicar() { return errors.New("no está lista") } // se rechaza: no se guarda nada
    i.Estado = modelo.EstadoPublicando                            // se acepta: se guarda
    return nil
})
```

**Escritura segura.** Cada cambio escribe primero `instancias.json.tmp` y luego lo renombra. El renombrado es atómico, así que el archivo nunca queda a medio escribir.

**Recuperación.** Si la app se cerró a mitad de una operación (`creando`, `publicando`...), al volver a abrir esa instancia pasa a `error` con un mensaje. La goroutine que la procesaba ya no existe, y así el usuario puede eliminarla.

## Formato del archivo

```json
[
  {
    "nombre": "paisajes",
    "dominio": "paisajes.cloud.local",
    "ip": "192.168.10.30",
    "mascara": 24,
    "nombre_vm": "paisajes",
    "estado": "publicada",
    "detalle": "Contenido publicado correctamente",
    "archivo": "sitio.zip",
    "creada_en": "2026-11-16T17:02:03-05:00",
    "publicada_en": "2026-11-16T17:05:10-05:00"
  }
]
```
