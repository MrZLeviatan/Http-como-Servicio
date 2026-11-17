# internal/aprovisionador — Orquestador

Es el **cerebro** del servicio: combina `virtualbox`, `remoto`, `dns` y `almacen` para crear, publicar y eliminar instancias. Responde rápido a la web y hace el trabajo largo en **goroutines** (segundo plano).

## API

| Método | Responde | En segundo plano |
|---|---|---|
| `SolicitarInstancia(nombre)` | Valida, reserva IP y registra en estado `creando` | `crear()` |
| `Publicar(nombre, zip, original)` | Pasa a `publicando` si está `lista`/`publicada` | `publicar()` |
| `Eliminar(nombre)` | Pasa a `eliminando` si no está ocupada | `eliminar()` |
| `Apagar(nombre)` | Pasa a `apagando` si está `lista`/`publicada` | `apagar()` |
| `Encender(nombre)` | Pasa a `encendiendo` si está `apagada` | `encender()` |
| `IniciarSincronizacion(ctx)` | — | Cada 15 s compara los estados con VirtualBox |
| `Esperar()` | Bloquea hasta que terminen todas las tareas (cierre ordenado) | — |

## Crear: paso a paso

| Paso | Acción | Estado mostrado |
|---|---|---|
| 0 | Verifica que **nada** responda en `192.168.10.20` (si no, se configuraría la VM equivocada) | creando |
| 1 | `virtualbox.Crear` con el disco multiconexión | creando |
| 2 | `virtualbox.Iniciar` (headless) | creando |
| 3 | Espera SSH en la IP temporal `.20` | creando |
| 4 | `sudo configurar-instancia.sh <nombre> <ip>` (tolera el corte por reinicio) | configurando |
| 5 | Espera SSH en la IP definitiva | configurando |
| 6 | Verifica que `hostname` == nombre pedido | configurando |
| 7 | `dns.Registrar` | configurando |
| 8 | `dns.Resolver` y comprueba la IP | configurando |
| ✔ | Lista para publicar | **lista** |

Si cualquier paso falla, el estado pasa a **error** con el motivo en `Detalle`. La VM no se borra para poder inspeccionarla; se elimina con el botón **Eliminar**.

## Publicar

1. `scp` del `.zip` a `/tmp/httppaas-<nombre>.zip` en la VM.
2. `sudo desplegar-sitio.sh <nombre> /tmp/httppaas-<nombre>.zip`.
3. Guarda el nombre del archivo y la fecha → **publicada**.

Si falla, vuelve a `lista`/`publicada` con el error en `Detalle`, porque la VM sigue sana y se puede reintentar. El `.zip` temporal del PC siempre se borra.

## Eliminar

1. `virtualbox.Eliminar` (apaga + `unregistervm --delete`).
2. `dns.Eliminar`. Si falla, solo se avisa: un registro huérfano no bloquea nada.
3. `almacen.Eliminar`, que libera la IP para otra instancia.

## Apagar y encender

Apagar **no borra nada**: la VM, su disco diferencial, su contenido, su IP y su registro DNS se conservan. Solo deja de responder.

| Acción | Pasos | Si falla |
|---|---|---|
| Apagar | `ApagarOrdenado` (ACPI, hasta 60 s; luego corte de energía) → `apagada` | Vuelve a `lista`/`publicada` con el error en `Detalle` |
| Encender | `startvm --type headless` → espera SSH en **su** IP (no usa la `.20`) → vuelve a `lista`/`publicada` | No arrancó: sigue `apagada`. Arrancó pero no responde: `error` |

Como al encender la VM ya tiene su IP definitiva, no necesita esperar turno con `muCreacion`.

## Sincronización con VirtualBox

Si alguien enciende o apaga una VM **desde VirtualBox**, el dashboard quedaría desactualizado. Cada 15 s, `sincronizar()` revisa las instancias en estado estable:

| Guardado | En VirtualBox | Nuevo estado |
|---|---|---|
| `lista` / `publicada` | apagada | `apagada` ("se apagó fuera de la aplicación") |
| `apagada` | `running` | `lista` / `publicada` ("se encendió fuera de la aplicación") |

Las instancias con una operación en curso o con `error` no se tocan. El cambio se hace con `almacen.Actualizar`, comprobando que el estado no haya cambiado entretanto, así que nunca pisa una operación que empezó a mitad de la revisión.

## Concurrencia

| Candado | Protege |
|---|---|
| `muReserva` | Elegir nombre e IP: dos solicitudes simultáneas nunca reciben la misma IP |
| `muCreacion` | **Una creación a la vez**: todas las instancias arrancan con la misma IP temporal `.20` |
| `almacen.Actualizar` | Cambios de estado con verificación atómica (no se puede publicar algo que se está eliminando) |

Varias solicitudes de creación quedan **en cola** con el mensaje "esperando turno"; publicar y eliminar no esperan a las creaciones.

## Asignación de IPs

`siguienteIP()` recorre `host_inicio`…`host_fin` y devuelve la primera IP que no use ninguna instancia registrada. Al eliminar una instancia, su IP queda libre otra vez.
