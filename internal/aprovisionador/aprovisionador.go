// Package aprovisionador es el ORQUESTADOR del servicio: une VirtualBox,
// SSH y DNS para crear, publicar y eliminar instancias.
//
// Las operaciones largas (crear una VM tarda minutos) se ejecutan en segundo
// plano (goroutines). La web responde de inmediato y el dashboard muestra el
// progreso leyendo el estado guardado en el almacén.
package aprovisionador

import (
	"context"       // Tiempo límite de cada operación.
	"errors"        // Crear errores simples.
	"fmt"           // Formatear mensajes.
	"log"           // Registrar en la terminal lo que va pasando.
	"os"            // Borrar el .zip temporal.
	"path/filepath" // Obtener solo el nombre de un archivo.
	"strconv"       // Convertir el número de host a texto.
	"strings"       // Comparar textos.
	"sync"          // Candados.
	"time"          // Fechas y plazos.

	"httppaas/internal/almacen"    // Registro de instancias.
	"httppaas/internal/config"     // Configuración.
	"httppaas/internal/dns"        // Bind9 / nsupdate.
	"httppaas/internal/ejecutor"   // Códigos de salida de programas.
	"httppaas/internal/modelo"     // Struct Instancia y estados.
	"httppaas/internal/remoto"     // SSH / SCP.
	"httppaas/internal/virtualbox" // VBoxManage.
)

// Rutas de los scripts instalados DENTRO de la plantilla (ver carpeta scripts/).
const (
	scriptConfigurar = "/usr/local/bin/configurar-instancia.sh"
	scriptDesplegar  = "/usr/local/bin/desplegar-sitio.sh"
)

// Tiempos máximos de cada operación completa.
const (
	limiteCreacion    = 15 * time.Minute
	limitePublicacion = 5 * time.Minute
	limiteEliminacion = 3 * time.Minute
)

// Aprovisionador coordina todos los módulos.
type Aprovisionador struct {
	cfg     *config.Config      // Configuración completa.
	almacen *almacen.Almacen    // Registro de instancias.
	vbox    *virtualbox.Cliente // Control de VirtualBox.
	ssh     *remoto.Cliente     // Comandos remotos.
	dns     *dns.Cliente        // Registros DNS.

	// muCreacion garantiza que se cree UNA instancia a la vez: todas arrancan
	// con la misma IP temporal de la plantilla (192.168.10.20) y, si dos
	// arrancaran juntas, chocarían en la red.
	muCreacion sync.Mutex

	// muReserva protege la elección de nombre e IP para que dos solicitudes
	// simultáneas no reciban la misma IP.
	muReserva sync.Mutex

	// procesos permite esperar a que terminen las goroutines al cerrar la app.
	procesos sync.WaitGroup
}

// Nuevo crea el Aprovisionador con sus dependencias.
func Nuevo(cfg *config.Config, alm *almacen.Almacen, vbox *virtualbox.Cliente, ssh *remoto.Cliente, dnsCliente *dns.Cliente) *Aprovisionador {
	return &Aprovisionador{cfg: cfg, almacen: alm, vbox: vbox, ssh: ssh, dns: dnsCliente}
}

// Esperar bloquea hasta que terminen todas las operaciones en segundo plano.
func (a *Aprovisionador) Esperar() {
	a.procesos.Wait()
}

// enSegundoPlano lanza `tarea` en una goroutine y la registra en el WaitGroup.
func (a *Aprovisionador) enSegundoPlano(tarea func()) {
	a.procesos.Add(1) // Una tarea más pendiente.
	go func() {
		defer a.procesos.Done() // Al terminar, una tarea menos.
		tarea()
	}()
}

// ───────────────────────────── CREAR ─────────────────────────────

// SolicitarInstancia valida el nombre, le reserva una IP, la registra con
// estado "creando" y lanza la creación en segundo plano.
// Responde rápido: el trabajo pesado ocurre en crear().
func (a *Aprovisionador) SolicitarInstancia(nombre string) (modelo.Instancia, error) {
	// Normaliza: sin espacios a los lados y en minúsculas.
	nombre = modelo.NormalizarNombre(nombre)

	// Valida el formato del hostname.
	if err := modelo.ValidarNombre(nombre); err != nil {
		return modelo.Instancia{}, err
	}
	// No se permiten nombres reservados (ej. "ns").
	if a.dns.EsReservado(nombre) {
		return modelo.Instancia{}, fmt.Errorf("el nombre '%s' está reservado", nombre)
	}

	// Desde aquí, una sola solicitud a la vez elige nombre e IP.
	a.muReserva.Lock()
	defer a.muReserva.Unlock()

	// El nombre no debe estar ya registrado en la aplicación.
	if _, err := a.almacen.Obtener(nombre); err == nil {
		return modelo.Instancia{}, fmt.Errorf("ya existe una instancia llamada '%s'", nombre)
	}

	// Tampoco debe existir una VM con ese nombre en VirtualBox.
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	existe, err := a.vbox.Existe(ctx, nombre)
	if err != nil {
		return modelo.Instancia{}, fmt.Errorf("no se pudo consultar VirtualBox: %w", err)
	}
	if existe {
		return modelo.Instancia{}, fmt.Errorf("ya hay una máquina virtual llamada '%s' en VirtualBox", nombre)
	}

	// Elige la primera IP libre del rango.
	ip, err := a.siguienteIP()
	if err != nil {
		return modelo.Instancia{}, err
	}

	// Arma el registro de la nueva instancia.
	inst := modelo.Instancia{
		Nombre:   nombre,
		Dominio:  a.dns.NombreCompleto(nombre),
		IP:       ip,
		Mascara:  a.cfg.Red.Mascara,
		NombreVM: nombre, // La VM se llama igual que el host (como pide el enunciado).
		Estado:   modelo.EstadoCreando,
		Detalle:  "En cola: esperando turno para crear la máquina virtual",
		CreadaEn: time.Now(),
	}
	// Lo guarda: desde este momento aparece en el dashboard.
	if err := a.almacen.Agregar(inst); err != nil {
		return modelo.Instancia{}, err
	}

	// Lanza la creación real en segundo plano.
	a.enSegundoPlano(func() { a.crear(inst) })
	log.Printf("[crear] %s solicitada con IP %s", nombre, ip)
	return inst, nil
}

// siguienteIP devuelve la primera IP del rango que no use ninguna instancia.
// Debe llamarse con muReserva tomado.
func (a *Aprovisionador) siguienteIP() (string, error) {
	// Conjunto (mapa) de IPs ocupadas.
	ocupadas := make(map[string]bool)
	for _, inst := range a.almacen.Listar() {
		ocupadas[inst.IP] = true
	}
	// Recorre del host_inicio al host_fin, ej. .30 → .250
	for n := a.cfg.Red.HostInicio; n <= a.cfg.Red.HostFin; n++ {
		ip := a.cfg.Red.Prefijo + strconv.Itoa(n) // Ej. "192.168.10." + "30"
		if !ocupadas[ip] {
			return ip, nil
		}
	}
	return "", errors.New("no quedan IPs libres en el rango configurado")
}

// crear hace todo el trabajo de levantar una instancia. Se ejecuta en
// segundo plano. Si algo falla, deja la instancia en estado "error" con el
// detalle, para que el usuario lo vea y pueda eliminarla.
func (a *Aprovisionador) crear(inst modelo.Instancia) {
	// Espera su turno: solo una creación a la vez (por la IP temporal compartida).
	a.muCreacion.Lock()
	defer a.muCreacion.Unlock()

	// Contexto con tiempo máximo para toda la creación.
	ctx, cancelar := context.WithTimeout(context.Background(), limiteCreacion)
	defer cancelar()

	// Ejecuta los pasos; si alguno falla, se marca el error.
	if err := a.pasosCreacion(ctx, inst); err != nil {
		log.Printf("[crear] %s FALLÓ: %v", inst.Nombre, err)
		a.marcar(inst.Nombre, modelo.EstadoError, "Error al crear: "+err.Error())
		return
	}
	log.Printf("[crear] %s lista en %s", inst.Nombre, inst.URL())
}

// pasosCreacion contiene la secuencia de creación, paso a paso.
func (a *Aprovisionador) pasosCreacion(ctx context.Context, inst modelo.Instancia) error {
	ipPlantilla := a.cfg.Red.IPPlantilla // Ej. 192.168.10.20

	// Paso 0: comprobación de seguridad. Si algo ya responde en la IP de la
	// plantilla (otra VM quedó encendida), no se continúa: se configuraría
	// la máquina equivocada.
	a.marcar(inst.Nombre, modelo.EstadoCreando, "Verificando que la IP temporal "+ipPlantilla+" esté libre")
	if a.ssh.Responde(ctx, ipPlantilla) {
		return fmt.Errorf("la IP temporal %s ya está en uso por otra máquina; apágala e intenta de nuevo", ipPlantilla)
	}

	// Paso 1: crear la VM con el disco multiconexión.
	a.marcar(inst.Nombre, modelo.EstadoCreando, "Creando la máquina virtual desde el disco multiconexión")
	if err := a.vbox.Crear(ctx, inst.NombreVM); err != nil {
		return err
	}

	// Paso 2: encenderla (sin ventana).
	a.marcar(inst.Nombre, modelo.EstadoCreando, "Encendiendo la máquina virtual")
	if err := a.vbox.Iniciar(ctx, inst.NombreVM); err != nil {
		return err
	}

	// Paso 3: esperar a que arranque Debian (responde SSH en la IP temporal).
	a.marcar(inst.Nombre, modelo.EstadoCreando, "Esperando a que arranque en "+ipPlantilla)
	if err := a.ssh.EsperarDisponible(ctx, ipPlantilla); err != nil {
		return err
	}

	// Paso 4: ejecutar el script de configuración (hostname, IP, Apache).
	// El script reinicia la VM al final, por lo que la conexión SSH puede
	// cortarse (código 255): eso es normal y se tolera.
	a.marcar(inst.Nombre, modelo.EstadoConfigurando, "Configurando hostname, IP "+inst.IP+" y Apache")
	comando := remoto.Comando("sudo", scriptConfigurar, inst.Nombre, inst.IP)
	if _, err := a.ssh.Ejecutar(ctx, ipPlantilla, comando); err != nil &&
		ejecutor.CodigoSalida(err) != remoto.CodigoErrorConexion {
		return fmt.Errorf("el script de configuración falló: %w", err)
	}

	// Paso 5: esperar a que vuelva con la IP definitiva.
	a.marcar(inst.Nombre, modelo.EstadoConfigurando, "Reiniciando con la IP definitiva "+inst.IP)
	if err := a.ssh.EsperarDisponible(ctx, inst.IP); err != nil {
		return err
	}

	// Paso 6: comprobar que es la máquina correcta (su hostname coincide).
	hostname, err := a.ssh.Ejecutar(ctx, inst.IP, "hostname")
	if err != nil {
		return err
	}
	if strings.TrimSpace(hostname) != inst.Nombre {
		return fmt.Errorf("en %s respondió '%s' en lugar de '%s'", inst.IP, hostname, inst.Nombre)
	}

	// Paso 7: registrar el nombre en el DNS (Bind9).
	a.marcar(inst.Nombre, modelo.EstadoConfigurando, "Registrando "+inst.Dominio+" en el DNS")
	if err := a.dns.Registrar(ctx, inst.Nombre, inst.IP); err != nil {
		return err
	}

	// Paso 8: verificar que el DNS ya responde con la IP correcta.
	ips, err := a.dns.Resolver(ctx, inst.Nombre)
	if err != nil {
		return fmt.Errorf("el DNS no resuelve %s: %w", inst.Dominio, err)
	}
	if len(ips) == 0 || ips[0] != inst.IP {
		return fmt.Errorf("el DNS resolvió %s a %v en lugar de %s", inst.Dominio, ips, inst.IP)
	}

	// Listo: la instancia funciona y espera contenido.
	a.marcar(inst.Nombre, modelo.EstadoLista, "Instancia lista. Publica un .zip para mostrar tu página.")
	return nil
}

// ──────────────────────────── PUBLICAR ────────────────────────────

// Publicar sube el .zip `rutaZip` (ya guardado en el PC) a la instancia
// `nombre` y lo despliega en segundo plano. `nombreOriginal` es el nombre
// con que el usuario subió el archivo (solo para mostrarlo).
// La función se encarga de borrar `rutaZip` cuando termina.
func (a *Aprovisionador) Publicar(nombre, rutaZip, nombreOriginal string) error {
	// Cambia el estado a "publicando" solo si la instancia está lista.
	inst, err := a.almacen.Actualizar(nombre, func(i *modelo.Instancia) error {
		if !i.PuedePublicar() {
			return fmt.Errorf("la instancia '%s' no está lista para publicar (estado: %s)", i.Nombre, i.Estado)
		}
		i.Estado = modelo.EstadoPublicando
		i.Detalle = "Subiendo " + nombreOriginal
		return nil
	})
	if err != nil {
		// No se va a usar el archivo: se borra.
		_ = os.Remove(rutaZip)
		return err
	}

	// El despliegue real va en segundo plano.
	a.enSegundoPlano(func() { a.publicar(inst, rutaZip, nombreOriginal) })
	log.Printf("[publicar] %s ← %s", nombre, nombreOriginal)
	return nil
}

// publicar copia el .zip a la VM y ejecuta el script de despliegue.
func (a *Aprovisionador) publicar(inst modelo.Instancia, rutaZip, nombreOriginal string) {
	// Pase lo que pase, al final se borra el .zip temporal del PC.
	defer os.Remove(rutaZip)

	// Contexto con tiempo máximo para la publicación.
	ctx, cancelar := context.WithTimeout(context.Background(), limitePublicacion)
	defer cancelar()

	// Ruta temporal dentro de la VM, ej. /tmp/httppaas-paisajes.zip
	rutaRemota := "/tmp/httppaas-" + inst.Nombre + ".zip"

	// Paso 1: copiar el archivo con scp.
	if err := a.ssh.Copiar(ctx, rutaZip, inst.IP, rutaRemota); err != nil {
		a.falloPublicacion(inst.Nombre, err)
		return
	}

	// Paso 2: descomprimir y publicar con el script de la plantilla.
	a.marcar(inst.Nombre, modelo.EstadoPublicando, "Descomprimiendo y publicando "+nombreOriginal)
	comando := remoto.Comando("sudo", scriptDesplegar, inst.Nombre, rutaRemota)
	if _, err := a.ssh.Ejecutar(ctx, inst.IP, comando); err != nil {
		a.falloPublicacion(inst.Nombre, err)
		return
	}

	// Éxito: se guarda qué archivo se publicó y cuándo.
	_, _ = a.almacen.Actualizar(inst.Nombre, func(i *modelo.Instancia) error {
		i.Estado = modelo.EstadoPublicada
		i.Detalle = "Contenido publicado correctamente"
		i.Archivo = filepath.Base(nombreOriginal)
		i.PublicadaEn = time.Now()
		return nil
	})
	log.Printf("[publicar] %s publicada (%s)", inst.Nombre, nombreOriginal)
}

// falloPublicacion registra un error de publicación. La VM sigue funcionando,
// así que vuelve a "lista" (o "publicada" si ya tenía contenido) para permitir
// reintentar, y el detalle explica qué pasó.
func (a *Aprovisionador) falloPublicacion(nombre string, err error) {
	log.Printf("[publicar] %s FALLÓ: %v", nombre, err)
	_, _ = a.almacen.Actualizar(nombre, func(i *modelo.Instancia) error {
		// Si nunca se publicó nada, vuelve a "lista"; si no, a "publicada".
		if i.PublicadaEn.IsZero() {
			i.Estado = modelo.EstadoLista
		} else {
			i.Estado = modelo.EstadoPublicada
		}
		i.Detalle = "Error al publicar: " + err.Error()
		return nil
	})
}

// ──────────────────────────── ELIMINAR ────────────────────────────

// Eliminar marca la instancia como "eliminando" y la borra en segundo plano:
// apaga y elimina la VM, borra el registro DNS y la quita del dashboard.
func (a *Aprovisionador) Eliminar(nombre string) error {
	// Solo se puede eliminar si no está a mitad de otra operación.
	inst, err := a.almacen.Actualizar(nombre, func(i *modelo.Instancia) error {
		if !i.PuedeEliminar() {
			return fmt.Errorf("la instancia '%s' está ocupada (estado: %s); espera a que termine", i.Nombre, i.Estado)
		}
		i.Estado = modelo.EstadoEliminando
		i.Detalle = "Apagando y eliminando la máquina virtual"
		return nil
	})
	if err != nil {
		return err
	}

	// La eliminación real va en segundo plano.
	a.enSegundoPlano(func() { a.eliminar(inst) })
	log.Printf("[eliminar] %s solicitada", nombre)
	return nil
}

// eliminar borra la VM, el registro DNS y la entrada del almacén.
func (a *Aprovisionador) eliminar(inst modelo.Instancia) {
	// Contexto con tiempo máximo para la eliminación.
	ctx, cancelar := context.WithTimeout(context.Background(), limiteEliminacion)
	defer cancelar()

	// Paso 1: apagar y borrar la VM (y su disco diferencial).
	if err := a.vbox.Eliminar(ctx, inst.NombreVM); err != nil {
		log.Printf("[eliminar] %s FALLÓ: %v", inst.Nombre, err)
		a.marcar(inst.Nombre, modelo.EstadoError, "Error al eliminar la VM: "+err.Error())
		return
	}

	// Paso 2: borrar el registro DNS. Si falla, solo se avisa en la terminal:
	// la VM ya no existe y un registro huérfano no impide nada.
	if err := a.dns.Eliminar(ctx, inst.Nombre); err != nil {
		log.Printf("[eliminar] %s: aviso, no se pudo borrar el DNS: %v", inst.Nombre, err)
	}

	// Paso 3: quitarla del registro (desaparece del dashboard y libera la IP).
	if err := a.almacen.Eliminar(inst.Nombre); err != nil {
		log.Printf("[eliminar] %s: no se pudo actualizar el registro: %v", inst.Nombre, err)
		return
	}
	log.Printf("[eliminar] %s eliminada", inst.Nombre)
}

// ──────────────────────────── AUXILIARES ────────────────────────────

// marcar cambia el estado y el detalle de una instancia (para mostrar progreso).
func (a *Aprovisionador) marcar(nombre string, estado modelo.Estado, detalle string) {
	_, err := a.almacen.Actualizar(nombre, func(i *modelo.Instancia) error {
		i.Estado = estado
		i.Detalle = detalle
		return nil
	})
	if err != nil {
		log.Printf("[estado] no se pudo actualizar %s: %v", nombre, err)
	}
}
