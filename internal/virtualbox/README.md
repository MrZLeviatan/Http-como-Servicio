# internal/virtualbox — Control de VirtualBox

Crea, arranca, apaga y elimina máquinas virtuales con `VBoxManage`, usando el **disco plantilla en modo multiconexión**.

## API

| Método | Comandos `VBoxManage` |
|---|---|
| `Existe(nombre)` | `list vms` |
| `Crear(nombre)` | `createvm` → `modifyvm` → `storagectl` → `storageattach` |
| `Iniciar(nombre)` | `startvm --type headless` |
| `Estado(nombre)` | `showvminfo --machinereadable` → `VMState` |
| `Apagar(nombre)` | `controlvm poweroff` (corte de energía) y espera a `poweroff` |
| `ApagarOrdenado(nombre, espera)` | `controlvm acpipowerbutton` (Debian se apaga solo); si no termina en `espera`, `Apagar` |
| `Encendida(nombre)` | `Estado` == `running` |
| `Eliminar(nombre)` | `Apagar` + `unregistervm --delete` |

## Cómo se crea una instancia

```bash
VBoxManage createvm --name paisajes --ostype Debian13_64 --basefolder <carpeta_vms> --register

VBoxManage modifyvm paisajes --memory 512 --cpus 1 --ioapic on --rtc-use-utc on \
  --graphicscontroller vmsvga --vram 16 --audio-enabled off --boot1 disk \
  --nic1 nat --nic-type1 82540EM \
  --nic2 hostonly --nic-type2 82540EM --host-only-adapter2 vboxnet0

VBoxManage storagectl paisajes --name SATA --add sata --controller IntelAhci --portcount 1 --bootable on

VBoxManage storageattach paisajes --storagectl SATA --port 0 --device 0 --type hdd \
  --medium "<disco_plantilla>.vdi"

VBoxManage startvm paisajes --type headless
```

El hardware **coincide con la plantilla** (mismo chipset y tarjetas Intel en los mismos puertos). Así Debian ve las interfaces con los mismos nombres (`enp0s3` = NAT, `enp0s8` = host-only) y su `/etc/network/interfaces` sigue siendo válido.

Si un paso falla, `Crear` borra la VM a medio crear (`unregistervm --delete`).

## Disco multiconexión

```
ApacheServer.vdi  (base, solo lectura, Type: multiattach)
   ├── {uuid}.vdi  ← diferencial de "paisajes"  (Instancias/paisajes/Snapshots/)
   ├── {uuid}.vdi  ← diferencial de "negocios"
   └── {uuid}.vdi  ← diferencial de la VM plantilla (no se usa)
```

- Al conectar el disco base, VirtualBox crea automáticamente un **disco diferencial** por VM. Cada VM solo escribe en el suyo.
- `unregistervm --delete` borra solo los discos conectados *directamente*, es decir, el diferencial. El base sigue conectado a la VM plantilla y VirtualBox no permite borrarlo.

## Notas

- La VM se llama igual que el hostname. `Existe` evita chocar con una VM que ya existe.
- Para **eliminar**, se usa `Apagar` (`poweroff`, inmediato), porque la VM se va a borrar de todas formas.
- Para el botón **Apagar**, se usa `ApagarOrdenado`: la señal ACPI deja que Debian cierre Apache y el disco correctamente. Solo si no responde en 60 s se corta la energía.
