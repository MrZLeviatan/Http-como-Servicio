# HTTPPaaS — Servicio administrado de servidores HTTP

**Computación en la nube 2026-2 · Nota parcial #3**

Plataforma local que simula un servicio de nube: un usuario escribe un nombre de host, sube un `.zip` con su página y el sistema, **sin intervención manual**:

1. Crea una máquina virtual Debian 13 con Apache a partir de un **disco multiconexión** (VirtualBox).
2. Le asigna hostname e IP propios (`paisajes` → `192.168.10.30`).
3. Registra `paisajes.cloud.local` en el **DNS Bind9**.
4. Despliega el contenido del `.zip` en Apache.
5. Lo muestra en un **dashboard** con su enlace, IP, hostname, fecha y botón **Eliminar**.

---

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

## Flujo de una solicitud

| Acción en la web | Qué ocurre | Estados |
|---|---|---|
| **Aceptar** (nombre del host) | Reserva IP → crea VM con el disco multiconexión → arranca en `.20` → ejecuta `configurar-instancia.sh` (hostname, IP, VirtualHost) → reinicia → verifica hostname en la IP nueva → `nsupdate` agrega el registro A → verifica la resolución | `creando` → `configurando` → `lista` |
| **Publicar** (.zip) | Valida el .zip → `scp` a la VM → `desplegar-sitio.sh` lo descomprime en `/var/www/<host>` → recarga Apache | `publicando` → `publicada` |
| **Eliminar** | Apaga la VM → `unregistervm --delete` (borra solo su disco diferencial) → `nsupdate` borra el registro → sale del dashboard | `eliminando` → (desaparece) |

Las operaciones largas corren en **segundo plano**; el dashboard se actualiza solo cada 4 s. Las creaciones se hacen **de una en una**, porque todas arrancan con la IP temporal `.20`.

---

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

## Requisitos previos

Fases 1 y 2 de la guía completadas:

- [x] Red host-only `vboxnet0` con `192.168.10.1/24`, sin DHCP.
- [x] VM DNS (`ns`, `192.168.10.10`) con Bind9, zona `cloud.local` y llave `ddns-key`.
- [x] Llave TSIG copiada al PC (`~/httppaas/ddns.key`).
- [x] Plantilla Apache en `192.168.10.20` con:
  - `apache2`, `unzip`, `sudo`
  - tu llave SSH en `authorized_keys`
  - `sudo` sin contraseña para el usuario
  - `/usr/local/bin/configurar-instancia.sh` y `/usr/local/bin/desplegar-sitio.sh`
- [x] Disco de la plantilla en modo **multiconexión** (`Type: multiattach`).
- [x] En el PC: Go ≥ 1.24, VirtualBox (`VBoxManage`), `ssh`, `scp`, `nsupdate` (`bind9-dnsutils`).

---

## Configuración

Edita [`config.json`](config.json). Los campos que **debes revisar**:

| Campo | Qué poner |
|---|---|
| `virtualbox.disco_plantilla` | Ruta exacta del `.vdi` multiconexión |
| `virtualbox.carpeta_vms` | Carpeta donde se guardarán las instancias |
| `ssh.usuario` | Usuario de la plantilla (`parcial2` o `parcial3`) |
| `ssh.llave_privada` | Llave cuya `.pub` copiaste a la plantilla |
| `dns.llave_tsig` | Ruta del `ddns.key` en tu PC |

Al arrancar, la app valida todo y te dice exactamente qué falta. Detalle de cada campo: [internal/config/README.md](internal/config/README.md).

---

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

### Probar el flujo completo

1. Escribe `paisajes` → **Aceptar**. Espera ~1-3 min hasta el estado **Lista**.
2. Crea un zip de prueba: `mkdir sitio && echo '<h1>Hola nube</h1>' > sitio/index.html && zip -r sitio.zip sitio`
3. Elige `paisajes.cloud.local`, selecciona `sitio.zip` → **Publicar**.
4. Abre el enlace `http://paisajes.cloud.local` (se abre en otra pestaña).
5. **Eliminar** → la VM desaparece de VirtualBox y del DNS.

---

## Solución de problemas

| Síntoma | Causa probable / solución |
|---|---|
| Al arrancar: `Configuración inválida` | Lee la lista: ruta o programa faltante en `config.json` |
| `la IP temporal 192.168.10.20 ya está en uso` | La plantilla u otra instancia quedó encendida en `.20`. Apágala |
| Se queda en `creando` y pasa a `error` por SSH | La VM no arrancó o tu llave no está en la plantilla. Mira la VM en VirtualBox → *Mostrar* |
| `el script de configuración falló` | Los scripts no están en `/usr/local/bin` o falta `sudo` sin contraseña |
| `no se pudo actualizar el DNS ... BADKEY` | `dns.llave_tsig` no corresponde a la llave de Bind9 |
| El enlace no abre en el navegador | Faltan los comandos `resolvectl` del paso 2 (se pierden al reiniciar el PC) |
| Error en VirtualBox al crear | Ver la terminal de la app: muestra el comando `VBoxManage` exacto que falló |

Los registros de cada operación aparecen en la terminal donde corre la app (`[crear]`, `[publicar]`, `[eliminar]`, `[web]`).

---

## Notas de seguridad (entorno de laboratorio)

- SSH no verifica la huella de las VMs (`StrictHostKeyChecking=no`), porque todas comparten el mismo disco y las IPs se reutilizan. En producción se usarían huellas únicas por VM.
- El usuario de las VMs tiene `sudo` sin contraseña para que la automatización funcione sin interacción.
- No incluyas la línea `secret` de `ddns.key` en capturas ni en el repositorio.
