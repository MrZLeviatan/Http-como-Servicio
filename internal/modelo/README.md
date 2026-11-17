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

Métodos: `URL()`, `IPConMascara()`, `EnProceso()`, `Activa()`, `EstadoActivo()`, `PuedePublicar()`, `PuedeApagar()`, `PuedeEncender()`, `PuedeEliminar()`.

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

Encendido y apagado (sin borrar la VM):

```
 lista / publicada ──Apagar──► apagando ──► apagada ──Encender──► encendiendo ──► lista / publicada
```

Al encender, la instancia vuelve al estado que tenía: `publicada` si alguna vez se publicó contenido (`EstadoActivo()`), o `lista` si no.

| Estado | ¿Publicar? | ¿Apagar? | ¿Encender? | ¿Eliminar? |
|---|---|---|---|---|
| `creando`, `configurando`, `publicando`, `apagando`, `encendiendo`, `eliminando` | No | No | No | No (en proceso) |
| `lista`, `publicada` | Sí | Sí | No | Sí |
| `apagada` | No | No | Sí | Sí |
| `error` | No | No | No | Sí |

## Validación de nombres

`ValidarNombre` aplica la regla de hostname DNS (la misma que `scripts/configurar-instancia.sh`):

- Solo `a-z`, `0-9` y `-`; empieza y termina con letra o número; máximo 63 caracteres.
- Válidos: `paisajes`, `mi-sitio2`. Inválidos: `Mi Sitio`, `-x`, `canción`.

`NormalizarNombre` quita espacios y pasa a minúsculas antes de validar.
