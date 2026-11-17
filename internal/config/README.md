# internal/config — Configuración

Lee `config.json`, rellena valores por defecto, expande rutas con `~` y **valida** todo antes de que la aplicación arranque.

## API

| Función / tipo | Descripción |
|---|---|
| `Cargar(ruta) (*Config, error)` | Carga, completa, expande y valida. Devuelve todos los problemas juntos |
| `ExpandirHome(ruta)` | `~/x` → `/home/usuario/x` |
| `Publicacion.TamanoMaximoBytes()` | Límite de subida convertido de MB a bytes |

## Campos de `config.json`

### `servidor`
| Campo | Por defecto | Descripción |
|---|---|---|
| `direccion` | `:8080` | Dirección y puerto de la webapp |

### `virtualbox`
| Campo | Por defecto | Descripción |
|---|---|---|
| `ejecutable` | `VBoxManage` | Programa de línea de comandos de VirtualBox |
| `disco_plantilla` | — **obligatorio** | Ruta del `.vdi` en modo multiconexión |
| `carpeta_vms` | — **obligatorio** | Dónde se crean las carpetas de las instancias |
| `tipo_so` | `Debian13_64` | Tipo de SO (`VBoxManage list ostypes`) |
| `memoria_mb` | `512` | RAM de cada instancia |
| `cpus` | `1` | Núcleos de cada instancia |
| `red_hostonly` | `vboxnet0` | Interfaz host-only del adaptador 2 |
| `controlador` | `SATA` | Nombre del controlador de disco |

### `red`
| Campo | Por defecto | Descripción |
|---|---|---|
| `ip_plantilla` | `192.168.10.20` | IP con la que arranca toda instancia antes de configurarse |
| `prefijo` | `192.168.10.` | Parte fija de las IPs |
| `host_inicio` / `host_fin` | `30` / `250` | Rango de IPs que se reparte |
| `mascara` | `24` | Solo para mostrar (`/24`) |

### `ssh`
| Campo | Por defecto | Descripción |
|---|---|---|
| `usuario` | — **obligatorio** | Usuario de la plantilla con `sudo` sin contraseña |
| `llave_privada` | `~/.ssh/id_ed25519` | Llave cuya parte pública está en la plantilla |
| `tiempo_conexion_seg` | `5` | Máximo por intento de conexión |
| `espera_arranque_seg` | `240` | Máximo esperando a que una VM arranque |

### `dns`
| Campo | Por defecto | Descripción |
|---|---|---|
| `ejecutable` | `nsupdate` | Programa de actualización dinámica |
| `servidor` | `192.168.10.10` | IP de la VM `ns` |
| `zona` | `cloud.local` | Zona administrada |
| `llave_tsig` | — **obligatorio** | Archivo `ddns.key` copiado de la VM `ns` |
| `ttl` | `300` | Duración de los registros en caché |
| `hosts_reservados` | `[]` | Nombres que no se pueden solicitar (ej. `ns`) |

### `almacenamiento` y `publicacion`
| Campo | Por defecto | Descripción |
|---|---|---|
| `almacenamiento.archivo` | `data/instancias.json` | Registro de instancias |
| `publicacion.tamano_maximo_mb` | `50` | Tamaño máximo del `.zip` |

## Validaciones

- El JSON no puede tener campos desconocidos (detecta errores de escritura).
- `disco_plantilla`, `llave_privada` y `llave_tsig` deben existir.
- IPs válidas y rango `1 ≤ inicio ≤ fin ≤ 254`.
- `VBoxManage`, `ssh`, `scp` y `nsupdate` deben estar instalados.
