# cmd/httppaas — Punto de entrada

Contiene `main.go`, el programa que se ejecuta. No tiene lógica de negocio: solo **arma** la aplicación conectando los módulos de `internal/`.

## Qué hace, en orden

| # | Paso | Módulo usado |
|---|---|---|
| 1 | Lee el argumento `-config` (por defecto `config.json`) | `flag` |
| 2 | Carga y valida la configuración; si algo falla, termina explicando qué corregir | `config` |
| 3 | Abre `data/instancias.json` | `almacen` |
| 4 | Crea los clientes de VirtualBox, SSH y DNS | `virtualbox`, `remoto`, `dns` |
| 5 | Crea el orquestador con esos clientes | `aprovisionador` |
| 6 | Crea la interfaz web y sus rutas | `servidor` |
| 7 | Arranca el servidor HTTP y muestra un resumen | `net/http` |
| 8 | Al recibir `Ctrl+C`: deja de aceptar peticiones y espera a que terminen las operaciones en segundo plano | `os/signal` |

## Uso

```bash
go run ./cmd/httppaas                     # usa config.json
go run ./cmd/httppaas -config otro.json   # otra configuración
go build -o bin/httppaas ./cmd/httppaas   # compilar un ejecutable
```

## Tiempos límite del servidor HTTP

| Campo | Valor | Motivo |
|---|---|---|
| `ReadHeaderTimeout` | 10 s | Evita conexiones que nunca envían cabeceras |
| `ReadTimeout` | 10 min | Permite subir un `.zip` grande |
| `WriteTimeout` | 30 s | Las respuestas son rápidas: el trabajo pesado va en segundo plano |
| `IdleTimeout` | 2 min | Cierra conexiones inactivas |

## Cierre ordenado

Con el primer `Ctrl+C` se espera a que terminen las creaciones, publicaciones y eliminaciones en curso, para no dejar VMs a medio configurar. Un segundo `Ctrl+C` cierra de inmediato. Lo que quede a medias se marca como `error` al volver a arrancar (ver `almacen`).
