# internal/aprovisionador — Orquestador

Es el **cerebro** del servicio: combina `virtualbox`, `remoto`, `dns` y `almacen` para crear, publicar y eliminar instancias. Responde rápido a la web y hace el trabajo largo en **goroutines** (segundo plano).

## API

| Método | Responde | En segundo plano |
|---|---|---|
| `SolicitarInstancia(nombre)` | Valida, reserva IP y registra en estado `creando` | `crear()` |
| `Publicar(nombre, zip, original)` | Pasa a `publicando` si está `lista`/`publicada` | `publicar()` |
| `Eliminar(nombre)` | Pasa a `eliminando` si no está ocupada | `eliminar()` |
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

## Concurrencia

| Candado | Protege |
|---|---|
| `muReserva` | Elegir nombre e IP: dos solicitudes simultáneas nunca reciben la misma IP |
| `muCreacion` | **Una creación a la vez**: todas las instancias arrancan con la misma IP temporal `.20` |
| `almacen.Actualizar` | Cambios de estado con verificación atómica (no se puede publicar algo que se está eliminando) |

Varias solicitudes de creación quedan **en cola** con el mensaje "esperando turno"; publicar y eliminar no esperan a las creaciones.

## Asignación de IPs

`siguienteIP()` recorre `host_inicio`…`host_fin` y devuelve la primera IP que no use ninguna instancia registrada. Al eliminar una instancia, su IP queda libre otra vez.
