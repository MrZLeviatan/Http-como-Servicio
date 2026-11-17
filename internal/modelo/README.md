# internal/modelo — Modelo de datos

Define qué es una **instancia** y su **ciclo de vida**. No depende de ningún otro módulo del proyecto.

## `Instancia`

| Campo | Ejemplo | Descripción |
|---|---|---|
| `Nombre` | `paisajes` | Hostname (también nombre de la VM) |
| `Dominio` | `paisajes.cloud.local` | Nombre completo en el DNS |
| `IP` / `Mascara` | `192.168.10.30` / `24` | Dirección asignada |
| `NombreVM` | `paisajes` | Nombre en VirtualBox |
| `Estado` | `lista` | Etapa actual |
| `Detalle` | `Instancia lista...` | Último mensaje de progreso o error |
| `Archivo` | `sitio.zip` | Último `.zip` publicado |
| `CreadaEn` / `PublicadaEn` | fechas | Momento de creación y de última publicación |

Métodos: `URL()`, `IPConMascara()`, `EnProceso()`, `PuedePublicar()`, `PuedeEliminar()`.

## Ciclo de vida

```
 Aceptar                                         Publicar
    │                                               │
    ▼                                               ▼
 creando ──► configurando ──► lista ──────────► publicando ──► publicada
    │              │            │  ▲                 │             │ ▲
    └──────┬───────┘            │  └──(falla)────────┘             │ └─ Publicar otra vez
           ▼                    │                                  │
         error ◄────────────────┼──────────────────────────────────┤
           │                    ▼                                  ▼
           └──────────────► eliminando ──► (sale del registro) ◄───┘
                     Eliminar
```

| Estado | ¿Publicar? | ¿Eliminar? |
|---|---|---|
| `creando`, `configurando`, `publicando`, `eliminando` | No | No (en proceso) |
| `lista`, `publicada` | Sí | Sí |
| `error` | No | Sí |

## Validación de nombres

`ValidarNombre` aplica la regla de hostname DNS (la misma que `scripts/configurar-instancia.sh`):

- Solo `a-z`, `0-9` y `-`; empieza y termina con letra o número; máximo 63 caracteres.
- Válidos: `paisajes`, `mi-sitio2`. Inválidos: `Mi Sitio`, `-x`, `canción`.

`NormalizarNombre` quita espacios y pasa a minúsculas antes de validar.
