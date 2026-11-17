# internal/dns — Registros en Bind9

Agrega y borra registros **A** en la zona `cloud.local` mediante **actualización dinámica** (`nsupdate` + llave TSIG). Nunca edita el archivo de zona a mano.

## API

| Método | Descripción |
|---|---|
| `Registrar(nombre, ip)` | Borra cualquier A previo y agrega `nombre.zona → ip` |
| `Eliminar(nombre)` | Borra el registro A (no falla si no existe) |
| `Resolver(nombre)` | Consulta **directamente** a Bind9 (no al DNS del PC) para verificar |
| `NombreCompleto(nombre)` | `paisajes` → `paisajes.cloud.local` |
| `EsReservado(nombre)` | ¿Está en `hosts_reservados`? |

## Qué se envía a `nsupdate`

```bash
nsupdate -k ~/httppaas/ddns.key -t 10 <<EOF
server 192.168.10.10
zone cloud.local
update delete paisajes.cloud.local. A
update add paisajes.cloud.local. 300 A 192.168.10.30
send
EOF
```

Las órdenes van juntas en un solo `send`, así que se aplican todas o ninguna.

## Requisitos en la VM `ns`

- Zona declarada con `update-policy { grant ddns-key zonesub ANY; };`
- Archivo de zona en `/var/lib/bind/` (Bind necesita escribir en él).
- La llave de `dns.llave_tsig` debe ser **la misma** que incluye `named.conf.local`.

## Errores comunes

| Mensaje | Causa |
|---|---|
| `NOTAUTH(BADKEY)` | Bind no conoce la llave (falta el `include` o la llave es otra) |
| `REFUSED` | La zona no tiene `update-policy` |
| `SERVFAIL` | Bind no puede escribir el journal (permisos de `/var/lib/bind`) |
| `connection timed out` | La VM `ns` está apagada o no hay red con `192.168.10.10` |
