# HTTPPaaS — Servicio administrado de servidores HTTP


Plataforma local que simula un servicio de nube: un usuario escribe un nombre de host, sube un `.zip` con su página y el sistema, **sin intervención manual**:

1. Crea una máquina virtual Debian 13 con Apache a partir de un **disco multiconexión** (VirtualBox).
2. Le asigna hostname e IP propios.
3. Registra `nombre.cloud.local` en el **DNS Bind9**.
4. Despliega el contenido del `.zip` en Apache.
5. Lo muestra en un **dashboard** con su enlace, IP, hostname, fecha y botón **Eliminar**.

<br>

<br>

> **Universidad del Quindío**  
> **Programa:** Ingeniería de Sistemas y Computación  
> **Curso:** Computación en la Nube (2026-2)  
> **Autor:** Nicolas Cabrera Serrano


---

<br>


## Arquitectura

```
            Tu PC (host Linux) — 192.168.10.1
  ┌──────────────────────────────────────────────────────┐
  │  Navegador ──► Webapp Go (:8080)                      │
  │                   │                                   │
  │     ┌─────────────┼───────────────┬───────────────┐   │
  │     ▼             ▼               ▼               ▼   │
  │  VBoxManage     ssh / scp       nsupdate      data/   │
  │  (crear VMs)   (configurar,    (registros    instancias│
  │                 desplegar)       DNS)          .json   │
  └─────┬─────────────┬───────────────┬───────────────────┘
        │             │               │     red host-only vboxnet0
        ▼             ▼               ▼     192.168.10.0/24
  ApacheServer.vdi  ┌──────────────┐  ┌──────────────────┐
  (multiconexión)──►│ paisajes .30 │  │ ns  .10 (Bind9)  │
        │           │ negocios .31 │  │ zona cloud.local │
        └──────────►│ ...          │  └──────────────────┘
                    └──────────────┘
```

| Componente | IP | Función |
|---|---|---|
| PC (host) | `192.168.10.1` | Ejecuta la webapp Go y VirtualBox |
| VM `ns` | `192.168.10.10` | Bind9, zona `cloud.local` con actualización dinámica (TSIG) |
| Plantilla `ApacheServer` | `192.168.10.20` (temporal) | Disco base multiconexión; nunca se enciende |
| Instancias | `192.168.10.30` – `.250` | Una VM Apache por página publicada |

---


<br>


## Flujo de una solicitud

| Acción en la web | Qué ocurre | Estados |
|---|---|---|
| **Aceptar** (nombre del host) | Reserva IP → crea VM con el disco multiconexión → arranca en `.20` → ejecuta `configurar-instancia.sh` (hostname, IP, VirtualHost) → reinicia → verifica hostname en la IP nueva → `nsupdate` agrega el registro A → verifica la resolución | `creando` → `configurando` → `lista` |
| **Publicar** (.zip) | Valida el .zip → `scp` a la VM → `desplegar-sitio.sh` lo descomprime en `/var/www/<host>` → recarga Apache | `publicando` → `publicada` |
| **Apagar** | Apagado ordenado (ACPI, hasta 60 s; si no, corte de energía). Se conservan disco, contenido, IP y DNS | `apagando` → `apagada` |
| **Encender** | Arranca la VM → espera SSH en su IP → vuelve a su estado anterior | `encendiendo` → `lista`/`publicada` |
| **Eliminar** | Apaga la VM → `unregistervm --delete` (borra solo su disco diferencial) → `nsupdate` borra el registro → sale del dashboard | `eliminando` → (desaparece) |
| **Publicar otra vez** en la misma instancia | `desplegar-sitio.sh` borra el contenido anterior de `/var/www/<host>` y pone el nuevo. Mismo nombre, IP y DNS | `publicando` → `publicada` |

Las operaciones largas corren en **segundo plano**; el dashboard se actualiza solo cada 4 s. Cada 15 s la app compara los estados con VirtualBox, así que si apagas o enciendes una VM desde VirtualBox el dashboard lo refleja. Las creaciones se hacen **de una en una**, porque todas arrancan con la IP temporal `.20`.

---

<br>

## Estructura del proyecto

```
Http como Servicio/
├── cmd/httppaas/          Punto de entrada (main.go): conecta los módulos y arranca la web
├── internal/
│   ├── config/            Carga y valida config.json
│   ├── modelo/            Struct Instancia, estados y validación de nombres
│   ├── almacen/           Persistencia de instancias en JSON (seguro para concurrencia)
│   ├── ejecutor/          Ejecución de programas externos con captura de errores
│   ├── virtualbox/        Crear / arrancar / apagar / eliminar VMs con VBoxManage
│   ├── remoto/            Comandos y copias por SSH/SCP
│   ├── dns/               Registros DNS con nsupdate + verificación
│   ├── aprovisionador/    Orquestador: une todo en crear / publicar / eliminar
│   └── servidor/          Handlers HTTP, subida de .zip, rutas
├── web/                   HTML, CSS y JS (incrustados en el binario con go:embed)
├── scripts/               Scripts que se instalan DENTRO de la plantilla Apache
├── config.json            Configuración de tu entorno
├── data/                  (se crea solo) instancias.json
└── bin/                   (se crea al compilar) ejecutable
```

Cada carpeta tiene su propio `README.md` con el detalle del módulo.

---

<br>

## Configuración

Edita [`config.json`](config.json). Los campos que **debes revisar**:

| Campo | Qué poner |
|---|---|
| `virtualbox.disco_plantilla` | Ruta exacta del `.vdi` multiconexión |
| `virtualbox.carpeta_vms` | Carpeta donde se guardarán las instancias |
| `ssh.usuario` | Usuario de la plantilla (`parcial2`) |
| `ssh.llave_privada` | Llave `.pub`. |
| `dns.llave_tsig` | Ruta del `ddns.key` en tu PC |

Al arrancar, la app valida todo y te dice exactamente qué falta. Detalle de cada campo: [internal/config/README.md](internal/config/README.md).

---

<br>

## Ejecución

```bash
# 1. VM DNS encendida (la plantilla debe quedar APAGADA)
VBoxManage startvm "Parcial #3 - Base DNS" --type headless

# 2. Que tu PC resuelva *.cloud.local con el DNS del proyecto
sudo resolvectl dns vboxnet0 192.168.10.10
sudo resolvectl domain vboxnet0 '~cloud.local'

# 3. Compilar y ejecutar
go build -o bin/httppaas ./cmd/httppaas
./bin/httppaas                     # o: go run ./cmd/httppaas
```

Abre **http://localhost:8080**. Detén con `Ctrl+C` (espera a que terminen las operaciones en curso; un segundo `Ctrl+C` fuerza el cierre).


---

