// Package config carga y valida la configuración de la aplicación desde un
// archivo JSON (por defecto config.json).
//
// Toda la información que depende de TU entorno (rutas de VirtualBox, IPs,
// usuario SSH, llaves) vive en ese archivo, para no tener que recompilar
// el programa cuando algo cambia.
package config

import (
	"encoding/json" // Leer el archivo JSON y convertirlo en structs de Go.
	"errors"        // Crear errores simples y unir varios errores en uno.
	"fmt"           // Formatear mensajes de error.
	"net"           // Validar que las IPs escritas sean IPs reales.
	"os"            // Leer archivos y consultar si existen.
	"os/exec"       // Comprobar que los programas externos estén instalados.
	"path/filepath" // Unir y limpiar rutas de archivos.
	"strings"       // Funciones de texto (prefijos, minúsculas...).
)

// Config agrupa todas las secciones del archivo config.json.
// Las etiquetas `json:"..."` indican el nombre de cada campo dentro del JSON.
type Config struct {
	Servidor       Servidor       `json:"servidor"`       // Dónde escucha la webapp.
	VirtualBox     VirtualBox     `json:"virtualbox"`     // Cómo crear las VMs.
	Red            Red            `json:"red"`            // Rango de IPs para las instancias.
	SSH            SSH            `json:"ssh"`            // Cómo conectarse a las VMs.
	DNS            DNS            `json:"dns"`            // Cómo actualizar Bind9.
	Almacenamiento Almacenamiento `json:"almacenamiento"` // Dónde se guarda el listado de instancias.
	Publicacion    Publicacion    `json:"publicacion"`    // Límites para subir archivos .zip.
}

// Servidor configura el servidor web de la aplicación de gestión.
type Servidor struct {
	Direccion string `json:"direccion"` // Ej. ":8080" → http://localhost:8080
}

// VirtualBox contiene lo necesario para crear VMs a partir del disco multiconexión.
type VirtualBox struct {
	Ejecutable     string `json:"ejecutable"`      // Programa VBoxManage (o su ruta completa).
	DiscoPlantilla string `json:"disco_plantilla"` // Ruta del .vdi en modo multiattach.
	CarpetaVMs     string `json:"carpeta_vms"`     // Carpeta donde se guardan las VMs nuevas.
	TipoSO         string `json:"tipo_so"`         // Tipo de SO de VirtualBox, ej. "Debian13_64".
	MemoriaMB      int    `json:"memoria_mb"`      // RAM de cada instancia, en MB.
	CPUs           int    `json:"cpus"`            // Núcleos de CPU de cada instancia.
	RedHostOnly    string `json:"red_hostonly"`    // Interfaz host-only, ej. "vboxnet0".
	Controlador    string `json:"controlador"`     // Nombre del controlador de disco, ej. "SATA".
}

// Red describe la red del proyecto (192.168.10.0/24) y qué IPs se reparten.
type Red struct {
	IPPlantilla string `json:"ip_plantilla"` // IP temporal con la que arranca toda instancia nueva.
	Prefijo     string `json:"prefijo"`      // Parte fija de la IP, ej. "192.168.10."
	HostInicio  int    `json:"host_inicio"`  // Primer número a repartir, ej. 30 → 192.168.10.30
	HostFin     int    `json:"host_fin"`     // Último número a repartir, ej. 250.
	Mascara     int    `json:"mascara"`      // Longitud de la máscara, ej. 24 (/24).
}

// SSH describe cómo entrar a las VMs de forma automática (sin contraseña).
type SSH struct {
	Usuario           string `json:"usuario"`             // Usuario dentro de las VMs (con sudo sin contraseña).
	LlavePrivada      string `json:"llave_privada"`       // Llave privada de tu PC, ej. ~/.ssh/id_ed25519
	TiempoConexionSeg int    `json:"tiempo_conexion_seg"` // Segundos máximos para cada intento de conexión.
	EsperaArranqueSeg int    `json:"espera_arranque_seg"` // Segundos máximos esperando a que una VM arranque.
}

// DNS describe el servidor Bind9 y la llave para actualizarlo con nsupdate.
type DNS struct {
	Ejecutable      string   `json:"ejecutable"`       // Programa nsupdate.
	Servidor        string   `json:"servidor"`         // IP del servidor DNS (VM ns).
	Zona            string   `json:"zona"`             // Zona DNS, ej. "cloud.local".
	LlaveTSIG       string   `json:"llave_tsig"`       // Archivo ddns.key copiado desde la VM ns.
	TTL             int      `json:"ttl"`              // Segundos que los clientes guardan la respuesta.
	HostsReservados []string `json:"hosts_reservados"` // Nombres que los usuarios NO pueden pedir.
}

// Almacenamiento indica dónde se guarda el estado de la aplicación.
type Almacenamiento struct {
	Archivo string `json:"archivo"` // Archivo JSON con las instancias, ej. data/instancias.json
}

// Publicacion limita lo que los usuarios pueden subir.
type Publicacion struct {
	TamanoMaximoMB int `json:"tamano_maximo_mb"` // Tamaño máximo del .zip, en MB.
}

// Cargar lee el archivo JSON de `ruta`, completa los valores por defecto,
// expande rutas con "~" y valida que todo tenga sentido.
// Si algo está mal, devuelve un error que explica QUÉ corregir.
func Cargar(ruta string) (*Config, error) {
	// Abre el archivo de configuración.
	archivo, err := os.Open(ruta)
	if err != nil {
		return nil, fmt.Errorf("no se pudo abrir la configuración '%s': %w", ruta, err)
	}
	// "defer" cierra el archivo automáticamente al salir de esta función.
	defer archivo.Close()

	// Prepara un lector de JSON sobre el archivo.
	decodificador := json.NewDecoder(archivo)
	// Si el JSON trae un campo que no existe en los structs, es un error
	// (así se detectan errores de escritura como "memoria_mbb").
	decodificador.DisallowUnknownFields()

	// Llena el struct Config con los datos del JSON.
	var cfg Config
	if err := decodificador.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config.json mal formado: %w", err)
	}

	// Rellena los campos que se dejaron vacíos.
	cfg.aplicarValoresPorDefecto()

	// Convierte "~/algo" en "/home/usuario/algo".
	if err := cfg.expandirRutas(); err != nil {
		return nil, err
	}

	// Revisa que los valores sean válidos y que los archivos existan.
	if err := cfg.validar(); err != nil {
		return nil, err
	}

	// Devuelve la configuración lista para usar.
	return &cfg, nil
}

// aplicarValoresPorDefecto pone un valor razonable en cada campo vacío.
func (c *Config) aplicarValoresPorDefecto() {
	// Función auxiliar: si el texto está vacío, usa el valor por defecto.
	texto := func(valor *string, porDefecto string) {
		if strings.TrimSpace(*valor) == "" {
			*valor = porDefecto
		}
	}
	// Función auxiliar: si el número es 0 o negativo, usa el valor por defecto.
	numero := func(valor *int, porDefecto int) {
		if *valor <= 0 {
			*valor = porDefecto
		}
	}

	texto(&c.Servidor.Direccion, ":8080")
	texto(&c.VirtualBox.Ejecutable, "VBoxManage")
	texto(&c.VirtualBox.TipoSO, "Debian13_64")
	texto(&c.VirtualBox.RedHostOnly, "vboxnet0")
	texto(&c.VirtualBox.Controlador, "SATA")
	numero(&c.VirtualBox.MemoriaMB, 512)
	numero(&c.VirtualBox.CPUs, 1)
	texto(&c.Red.IPPlantilla, "192.168.10.20")
	texto(&c.Red.Prefijo, "192.168.10.")
	numero(&c.Red.HostInicio, 30)
	numero(&c.Red.HostFin, 250)
	numero(&c.Red.Mascara, 24)
	texto(&c.SSH.LlavePrivada, "~/.ssh/id_ed25519")
	numero(&c.SSH.TiempoConexionSeg, 5)
	numero(&c.SSH.EsperaArranqueSeg, 240)
	texto(&c.DNS.Ejecutable, "nsupdate")
	texto(&c.DNS.Servidor, "192.168.10.10")
	texto(&c.DNS.Zona, "cloud.local")
	numero(&c.DNS.TTL, 300)
	texto(&c.Almacenamiento.Archivo, "data/instancias.json")
	numero(&c.Publicacion.TamanoMaximoMB, 50)

	// El prefijo siempre debe terminar en "." para poder pegarle el número.
	if !strings.HasSuffix(c.Red.Prefijo, ".") {
		c.Red.Prefijo += "."
	}
	// La zona se guarda sin punto final y en minúsculas: "cloud.local".
	c.DNS.Zona = strings.ToLower(strings.TrimSuffix(c.DNS.Zona, "."))
}

// expandirRutas reemplaza "~" por la carpeta personal del usuario en las
// rutas de la configuración (Go, a diferencia de la terminal, no lo hace solo).
func (c *Config) expandirRutas() error {
	// Lista de punteros a cada campo que es una ruta.
	rutas := []*string{
		&c.VirtualBox.DiscoPlantilla,
		&c.VirtualBox.CarpetaVMs,
		&c.SSH.LlavePrivada,
		&c.DNS.LlaveTSIG,
		&c.Almacenamiento.Archivo,
	}
	// Recorre cada ruta y la expande.
	for _, ruta := range rutas {
		expandida, err := ExpandirHome(*ruta)
		if err != nil {
			return err
		}
		*ruta = expandida
	}
	return nil
}

// ExpandirHome convierte "~/x" en "<carpeta personal>/x". Otras rutas no cambian.
func ExpandirHome(ruta string) (string, error) {
	// Si no empieza con "~", se devuelve igual.
	if ruta != "~" && !strings.HasPrefix(ruta, "~/") {
		return ruta, nil
	}
	// Obtiene la carpeta personal, ej. /home/mrz_leviatan
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no se pudo obtener la carpeta personal: %w", err)
	}
	// Une la carpeta personal con el resto de la ruta (sin el "~").
	return filepath.Join(home, strings.TrimPrefix(ruta, "~")), nil
}

// validar revisa la configuración y devuelve TODOS los problemas encontrados
// juntos, para poder corregirlos de una sola vez.
func (c *Config) validar() error {
	// Aquí se acumulan los errores.
	var problemas []error

	// Función auxiliar para agregar un problema con formato.
	agregar := func(formato string, args ...any) {
		problemas = append(problemas, fmt.Errorf(formato, args...))
	}

	// --- VirtualBox ---
	if c.VirtualBox.DiscoPlantilla == "" {
		agregar("virtualbox.disco_plantilla es obligatorio (ruta del .vdi multiconexión)")
	} else if _, err := os.Stat(c.VirtualBox.DiscoPlantilla); err != nil {
		agregar("virtualbox.disco_plantilla: no existe '%s'", c.VirtualBox.DiscoPlantilla)
	}
	if c.VirtualBox.CarpetaVMs == "" {
		agregar("virtualbox.carpeta_vms es obligatorio (dónde se guardan las instancias)")
	}

	// --- Red ---
	if net.ParseIP(c.Red.IPPlantilla) == nil {
		agregar("red.ip_plantilla no es una IP válida: '%s'", c.Red.IPPlantilla)
	}
	if net.ParseIP(c.Red.Prefijo+"1") == nil {
		agregar("red.prefijo no es válido: '%s' (ej. \"192.168.10.\")", c.Red.Prefijo)
	}
	if c.Red.HostInicio < 1 || c.Red.HostFin > 254 || c.Red.HostInicio > c.Red.HostFin {
		agregar("red.host_inicio/host_fin deben cumplir 1 <= inicio <= fin <= 254")
	}

	// --- SSH ---
	if c.SSH.Usuario == "" {
		agregar("ssh.usuario es obligatorio (usuario de las VMs con sudo sin contraseña)")
	}
	if _, err := os.Stat(c.SSH.LlavePrivada); err != nil {
		agregar("ssh.llave_privada: no existe '%s'", c.SSH.LlavePrivada)
	}

	// --- DNS ---
	if net.ParseIP(c.DNS.Servidor) == nil {
		agregar("dns.servidor no es una IP válida: '%s'", c.DNS.Servidor)
	}
	if c.DNS.LlaveTSIG == "" {
		agregar("dns.llave_tsig es obligatorio (archivo ddns.key)")
	} else if _, err := os.Stat(c.DNS.LlaveTSIG); err != nil {
		agregar("dns.llave_tsig: no existe '%s'", c.DNS.LlaveTSIG)
	}

	// --- Programas externos que la app necesita ---
	for _, programa := range []string{c.VirtualBox.Ejecutable, "ssh", "scp", c.DNS.Ejecutable} {
		// LookPath busca el programa en el PATH (como hace la terminal).
		if _, err := exec.LookPath(programa); err != nil {
			agregar("no se encontró el programa '%s' (¿está instalado?)", programa)
		}
	}

	// errors.Join une todos los problemas en un solo error (nil si no hay ninguno).
	return errors.Join(problemas...)
}

// TamanoMaximoBytes devuelve el límite de subida convertido de MB a bytes.
func (p Publicacion) TamanoMaximoBytes() int64 {
	// 1 MB = 1024 * 1024 bytes; "<< 20" es lo mismo que multiplicar por 1.048.576.
	return int64(p.TamanoMaximoMB) << 20
}
