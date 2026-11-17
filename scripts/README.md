# scripts — Scripts de la plantilla Apache

Estos scripts **no se ejecutan en tu PC**. Se instalan **dentro de la plantilla** (`/usr/local/bin/`) y quedan en cada instancia clonada. La webapp los invoca por SSH con `sudo`.

| Script | Lo llama | Qué hace |
|---|---|---|
| `configurar-instancia.sh <host> <ip>` | Paso *configurar* al crear | Hostname, `/etc/hosts`, `/var/www/<host>` con página provisional, VirtualHost `<host>.cloud.local`, IP definitiva y reinicio en 2 s |
| `desplegar-sitio.sh <host> <zip>` | **Publicar** | Descomprime en un temporal, usa la carpeta interior si el zip trae una sola, reemplaza `/var/www/<host>`, ajusta dueño y permisos, borra el zip y recarga Apache |

Ambos están comentados línea por línea.

## Instalación en la plantilla (una vez)

```bash
# En tu PC
scp scripts/configurar-instancia.sh scripts/desplegar-sitio.sh parcial2@192.168.10.20:/tmp/

# Dentro de la plantilla (por SSH)
sudo install -m 755 /tmp/configurar-instancia.sh /tmp/desplegar-sitio.sh /usr/local/bin/
```

> ⚠️ No los ejecutes en la plantilla: cambiarían su nombre e IP.

## Requisitos dentro de la plantilla

- Paquetes `apache2`, `unzip` y `sudo`.
- Usuario con `sudo` sin contraseña (`/etc/sudoers.d/<usuario>`).
- En `/etc/network/interfaces`, la interfaz `enp0s8` con **una sola** línea `address <ip>/24`, que es la que reemplaza el script.

## Modificar un script después de convertir el disco a multiconexión

El disco base es de solo lectura para las instancias. Para cambiar un script:

1. Elimina todas las instancias.
2. Vuelve el disco a tipo **Normal** (Administrador de medios).
3. Enciende la plantilla, reinstala el script y apágala.
4. Vuelve el disco a **Multiconexión**.
