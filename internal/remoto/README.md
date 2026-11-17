# internal/remoto — SSH y SCP

Ejecuta comandos y copia archivos en las VMs con los programas `ssh` y `scp` del sistema, autenticando con la llave privada configurada (sin contraseña).

## API

| Función | Equivale a |
|---|---|
| `Ejecutar(ip, comando)` | `ssh <opciones> usuario@ip 'comando'` |
| `Copiar(origen, ip, destino)` | `scp <opciones> origen usuario@ip:destino` |
| `Responde(ip)` | Un intento de `ssh ... true` |
| `EsperarDisponible(ip)` | Reintenta cada 3 s hasta que responda o venza `espera_arranque_seg` |
| `Comando(partes...)` | Arma `'parte1' 'parte2' ...` con comillas seguras para la shell remota |

## Opciones de SSH

| Opción | Motivo |
|---|---|
| `-i <llave>` + `IdentitiesOnly=yes` | Usar solo esa llave (evita *Too many authentication failures*) |
| `BatchMode=yes` | Nunca pedir contraseña: falla en su lugar (la app no es interactiva) |
| `StrictHostKeyChecking=no` + `UserKnownHostsFile=/dev/null` | Todas las VMs salen del mismo disco (misma huella) y las IPs se reutilizan. **Solo para laboratorio** |
| `ConnectTimeout` | Máximo por intento (`tiempo_conexion_seg`) |
| `ServerAliveInterval=5` + `ServerAliveCountMax=2` | Detectar en ~10 s que la VM se reinició, en vez de quedarse colgado |

## Por qué `ssh`/`scp` del sistema y no una librería Go

- Usa la misma llave y configuración que ya probaste en la terminal.
- No requiere dependencias externas (`go.mod` sin `require`).
- Si falla, puedes copiar el mismo comando y probarlo a mano.

## Código 255

`ssh` termina con **255** cuando falla la *conexión* (no el comando remoto). `configurar-instancia.sh` reinicia la VM al final, así que ese corte es esperado. El orquestador lo tolera con `remoto.CodigoErrorConexion`.
