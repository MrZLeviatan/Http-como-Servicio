// Package virtualbox controla Oracle VirtualBox mediante el programa
// VBoxManage: crear, arrancar, apagar y eliminar máquinas virtuales.
//
// Cada VM nueva se conecta al disco plantilla en modo MULTICONEXIÓN: VirtualBox
// crea automáticamente un disco "diferencial" propio para la VM, donde se
// guardan solo sus cambios. El disco base nunca se modifica.
package virtualbox

import (
	"bufio"   // Leer la salida de VBoxManage línea por línea.
	"context" // Tiempo límite y cancelación.
	"fmt"     // Formatear argumentos y errores.
	"os"      // Crear la carpeta de las VMs.
	"strconv" // Convertir números a texto.
	"strings" // Procesar texto de la salida.
	"time"    // Pausas entre consultas de estado.

	"httppaas/internal/config"   // Configuración de VirtualBox.
	"httppaas/internal/ejecutor" // Ejecutar VBoxManage.
)

// Cliente ejecuta comandos de VBoxManage con la configuración del proyecto.
type Cliente struct {
	cfg config.VirtualBox // Sección "virtualbox" de config.json.
}

// Nuevo crea un Cliente de VirtualBox.
func Nuevo(cfg config.VirtualBox) *Cliente {
	return &Cliente{cfg: cfg}
}

// vbox ejecuta "VBoxManage <args...>" y devuelve su salida.
func (c *Cliente) vbox(ctx context.Context, args ...string) (string, error) {
	return ejecutor.Ejecutar(ctx, nil, c.cfg.Ejecutable, args...)
}

// Existe indica si ya hay una VM registrada en VirtualBox con ese nombre.
func (c *Cliente) Existe(ctx context.Context, nombre string) (bool, error) {
	// "VBoxManage list vms" imprime una línea por VM: "nombre" {uuid}
	salida, err := c.vbox(ctx, "list", "vms")
	if err != nil {
		return false, err
	}
	// Recorre la salida línea por línea.
	lector := bufio.NewScanner(strings.NewReader(salida))
	for lector.Scan() {
		// Cada línea empieza con el nombre entre comillas: "paisajes" {...}
		if strings.HasPrefix(lector.Text(), strconv.Quote(nombre)+" ") {
			return true, nil
		}
	}
	return false, nil
}

// Crear registra una VM nueva llamada `nombre`, con su hardware y el disco
// plantilla conectado en modo multiconexión. NO la arranca.
// Si algo falla a mitad, borra lo que alcanzó a crear.
func (c *Cliente) Crear(ctx context.Context, nombre string) (err error) {
	// Asegura que exista la carpeta donde se guardan las VMs.
	if err := os.MkdirAll(c.cfg.CarpetaVMs, 0o755); err != nil {
		return fmt.Errorf("no se pudo crear la carpeta de VMs: %w", err)
	}

	// 1. Crear y registrar la VM (por ahora vacía, sin hardware configurado).
	//    --basefolder: carpeta donde VirtualBox guarda los archivos de la VM.
	//    --register:   que aparezca en la lista de VirtualBox.
	if _, err := c.vbox(ctx, "createvm",
		"--name", nombre,
		"--ostype", c.cfg.TipoSO,
		"--basefolder", c.cfg.CarpetaVMs,
		"--register",
	); err != nil {
		return err
	}

	// Si cualquier paso siguiente falla, se elimina la VM a medio crear.
	// (Este "defer" se ejecuta al salir de la función y revisa el error final.)
	defer func() {
		if err != nil {
			// Usa un contexto nuevo: el original pudo haberse cancelado.
			limpieza, cancelar := context.WithTimeout(context.Background(), time.Minute)
			defer cancelar()
			_, _ = c.vbox(limpieza, "unregistervm", nombre, "--delete")
		}
	}()

	// 2. Configurar el hardware. Debe coincidir con la plantilla para que
	//    Debian vea las mismas tarjetas de red (enp0s3 y enp0s8).
	if _, err = c.vbox(ctx, "modifyvm", nombre,
		"--memory", strconv.Itoa(c.cfg.MemoriaMB), // RAM en MB.
		"--cpus", strconv.Itoa(c.cfg.CPUs), // Núcleos de CPU.
		"--ioapic", "on", // Necesario para más de 1 CPU.
		"--rtc-use-utc", "on", // Reloj en UTC (como Linux espera).
		"--graphicscontroller", "vmsvga", // Controlador gráfico estándar para Linux.
		"--vram", "16", // Memoria de video mínima (la VM no usa escritorio).
		"--audio-enabled", "off", // Sin sonido: no se necesita.
		"--boot1", "disk", "--boot2", "none", "--boot3", "none", "--boot4", "none", // Arrancar solo desde el disco.
		"--nic1", "nat", "--nic-type1", "82540EM", // Adaptador 1: NAT (salida a internet) → enp0s3.
		"--nic2", "hostonly", "--nic-type2", "82540EM", // Adaptador 2: red del proyecto → enp0s8.
		"--host-only-adapter2", c.cfg.RedHostOnly, // ...conectado a vboxnet0.
	); err != nil {
		return err
	}

	// 3. Agregar el controlador de disco (SATA), igual que en la plantilla.
	if _, err = c.vbox(ctx, "storagectl", nombre,
		"--name", c.cfg.Controlador, // Nombre del controlador, ej. "SATA".
		"--add", "sata", // Tipo de bus.
		"--controller", "IntelAhci", // Modelo de controlador SATA.
		"--portcount", "1", // Solo se necesita un puerto (un disco).
		"--bootable", "on", // Se puede arrancar desde él.
	); err != nil {
		return err
	}

	// 4. Conectar el disco plantilla (multiconexión). VirtualBox crea solo
	//    el disco diferencial de esta VM dentro de su carpeta "Snapshots".
	if _, err = c.vbox(ctx, "storageattach", nombre,
		"--storagectl", c.cfg.Controlador, // Controlador creado en el paso 3.
		"--port", "0", "--device", "0", // Primer puerto, primer dispositivo.
		"--type", "hdd", // Es un disco duro.
		"--medium", c.cfg.DiscoPlantilla, // Ruta del .vdi multiconexión.
	); err != nil {
		return err
	}

	// Todo salió bien: err es nil y el defer no borra nada.
	return nil
}

// Iniciar arranca la VM sin ventana (headless: corre en segundo plano).
func (c *Cliente) Iniciar(ctx context.Context, nombre string) error {
	_, err := c.vbox(ctx, "startvm", nombre, "--type", "headless")
	return err
}

// Estado devuelve el estado de la VM según VirtualBox: "running", "poweroff",
// "aborted", "saved", "paused", etc.
func (c *Cliente) Estado(ctx context.Context, nombre string) (string, error) {
	// --machinereadable imprime líneas "clave=valor", fáciles de procesar.
	salida, err := c.vbox(ctx, "showvminfo", nombre, "--machinereadable")
	if err != nil {
		return "", err
	}
	// Busca la línea VMState="running"
	lector := bufio.NewScanner(strings.NewReader(salida))
	for lector.Scan() {
		linea := lector.Text()
		if valor, ok := strings.CutPrefix(linea, "VMState="); ok {
			// Quita las comillas del valor.
			return strings.Trim(valor, `"`), nil
		}
	}
	return "", fmt.Errorf("VBoxManage no informó el estado de '%s'", nombre)
}

// Apagar corta la energía de la VM (como desenchufarla) y espera a que
// VirtualBox confirme que quedó apagada. Si ya estaba apagada, no hace nada.
func (c *Cliente) Apagar(ctx context.Context, nombre string) error {
	// Consulta el estado actual.
	estado, err := c.Estado(ctx, nombre)
	if err != nil {
		return err
	}
	// Estados en los que la VM ya no está corriendo.
	if apagada(estado) {
		return nil
	}

	// Apaga la VM. Se usa "poweroff" (inmediato) porque la instancia se va a
	// borrar de todas formas y así no hay que esperar a que Debian se apague.
	if _, err := c.vbox(ctx, "controlvm", nombre, "poweroff"); err != nil {
		return err
	}

	// Espera hasta que VirtualBox informe que está apagada (máx. según ctx).
	for {
		estado, err := c.Estado(ctx, nombre)
		if err == nil && apagada(estado) {
			return nil
		}
		// select espera lo que ocurra primero: que venza ctx o que pase 1 segundo.
		select {
		case <-ctx.Done():
			return fmt.Errorf("la VM '%s' no se apagó a tiempo: %w", nombre, ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

// Eliminar apaga la VM y la borra de VirtualBox junto con su disco diferencial.
//
// "unregistervm --delete" solo borra los discos conectados DIRECTAMENTE a la
// VM, es decir, su diferencial. El disco plantilla (el padre) no se toca: además
// sigue conectado a la VM plantilla, así que VirtualBox no permitiría borrarlo.
func (c *Cliente) Eliminar(ctx context.Context, nombre string) error {
	// Si la VM ya no existe en VirtualBox, no hay nada que hacer.
	existe, err := c.Existe(ctx, nombre)
	if err != nil {
		return err
	}
	if !existe {
		return nil
	}
	// Primero se apaga (no se puede borrar una VM encendida).
	if err := c.Apagar(ctx, nombre); err != nil {
		return err
	}
	// VirtualBox tarda un instante en liberar la sesión tras apagar la VM.
	time.Sleep(2 * time.Second)
	// Se elimina el registro de la VM y sus archivos.
	_, err = c.vbox(ctx, "unregistervm", nombre, "--delete")
	return err
}

// apagada indica si un estado de VirtualBox significa que la VM no corre.
func apagada(estado string) bool {
	switch estado {
	case "poweroff", "aborted", "saved":
		return true
	default:
		return false
	}
}
