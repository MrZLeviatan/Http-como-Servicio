# internal/ejecutor — Ejecución de programas externos

Punto único por el que la aplicación lanza `VBoxManage`, `ssh`, `scp` y `nsupdate`.

## API

| Función | Descripción |
|---|---|
| `Ejecutar(ctx, entrada, programa, args...)` | Ejecuta, espera y devuelve stdout. Si falla, el error incluye el comando completo y lo que el programa escribió en stderr |
| `CodigoSalida(err)` | Código de salida del programa (`-1` si no aplica) |

## Por qué existe

- **Errores útiles.** En vez de `exit status 1`, el error dice, por ejemplo:
  `VBoxManage storageattach paisajes ...: exit status 1: VBoxManage: error: Could not find file ...`
- **Tiempo límite.** Usa `exec.CommandContext`: si el contexto vence, el programa se termina.
- **Entrada estándar.** Permite enviar texto al programa, como las órdenes de `nsupdate`.
- **Códigos de salida.** `ssh` devuelve `255` cuando falla la *conexión*. El orquestador lo usa para tolerar el corte esperado cuando la VM se reinicia.

## Seguridad

Los argumentos se pasan como lista (`exec.Command(programa, args...)`), **sin pasar por una shell local**. Un nombre con caracteres raros no puede ejecutar otros comandos en el PC.
