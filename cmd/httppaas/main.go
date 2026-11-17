// Comando httppaas: punto de entrada de la aplicación web de gestión del
// servicio HTTPPaaS (HTTP como servicio).
//
// Carga la configuración, crea cada módulo, los conecta entre sí y arranca
// el servidor web. Al presionar Ctrl+C se detiene de forma ordenada.
//
// Uso:
//
//	go run ./cmd/httppaas                    (usa config.json)
//	go run ./cmd/httppaas -config otro.json
package main

import (
	"context"   // Contexto que se cancela al presionar Ctrl+C.
	"errors"    // Comparar el error de servidor cerrado.
	"flag"      // Leer argumentos de línea de comandos (-config).
	"fmt"       // Imprimir el resumen de arranque.
	"log"       // Registrar mensajes y errores fatales.
	"net/http"  // Servidor HTTP.
	"os"        // Señales del sistema operativo.
	"os/signal" // Capturar Ctrl+C.
	"syscall"   // Señal SIGTERM (cierre del sistema).
	"time"      // Tiempos límite del servidor.

	"httppaas/internal/almacen"        // Registro de instancias (JSON).
	"httppaas/internal/aprovisionador" // Orquestador.
	"httppaas/internal/config"         // Configuración.
	"httppaas/internal/dns"            // Bind9 / nsupdate.
	"httppaas/internal/remoto"         // SSH / SCP.
	"httppaas/internal/servidor"       // Aplicación web.
	"httppaas/internal/virtualbox"     // VBoxManage.
)

func main() {
	// Define el argumento -config con su valor por defecto y su descripción.
	rutaConfig := flag.String("config", "config.json", "ruta del archivo de configuración")
	// Lee los argumentos escritos al ejecutar el programa.
	flag.Parse()

	// 1. Cargar y validar la configuración. Si falla, el programa termina
	//    mostrando qué corregir.
	cfg, err := config.Cargar(*rutaConfig)
	if err != nil {
		log.Fatalf("Configuración inválida:\n%v", err)
	}

	// 2. Abrir el almacén de instancias (data/instancias.json).
	alm, err := almacen.Abrir(cfg.Almacenamiento.Archivo)
	if err != nil {
		log.Fatalf("No se pudo abrir el almacén: %v", err)
	}

	// 3. Crear los clientes de cada tecnología.
	vbox := virtualbox.Nuevo(cfg.VirtualBox) // VirtualBox.
	ssh := remoto.Nuevo(cfg.SSH)             // SSH/SCP.
	dnsCliente := dns.Nuevo(cfg.DNS)         // DNS.

	// 4. Crear el orquestador, que usa todos los anteriores.
	aprov := aprovisionador.Nuevo(cfg, alm, vbox, ssh, dnsCliente)

	// 5. Crear la aplicación web.
	web, err := servidor.Nuevo(cfg, alm, aprov)
	if err != nil {
		log.Fatalf("No se pudo preparar la interfaz web: %v", err)
	}

	// 6. Configurar el servidor HTTP con tiempos límite razonables.
	srv := &http.Server{
		Addr:              cfg.Servidor.Direccion, // Ej. ":8080".
		Handler:           web.Rutas(),            // Rutas de la aplicación.
		ReadHeaderTimeout: 10 * time.Second,       // Máx. para recibir las cabeceras.
		ReadTimeout:       10 * time.Minute,       // Máx. para recibir la petición (subir un .zip grande).
		WriteTimeout:      30 * time.Second,       // Máx. para enviar la respuesta.
		IdleTimeout:       2 * time.Minute,        // Máx. de una conexión inactiva.
	}

	// 7. Preparar el cierre ordenado: este contexto se cancela con Ctrl+C
	//    (SIGINT) o cuando el sistema pide cerrar el programa (SIGTERM).
	ctx, detener := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer detener()

	// Revisa cada 15 s si alguna VM se encendió o apagó desde VirtualBox
	// y corrige su estado en el dashboard. Se detiene con Ctrl+C.
	aprov.IniciarSincronizacion(ctx)

	// 8. Arrancar el servidor en una goroutine (para que main pueda esperar Ctrl+C).
	go func() {
		// ListenAndServe bloquea hasta que el servidor se cierra.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Error crítico en el servidor: %v", err)
		}
	}()

	// Resumen de arranque en la terminal.
	fmt.Println("HTTPPaaS — SERVICIO ADMINISTRADO DE SERVIDORES HTTP")
	fmt.Println("Configuración cargada correctamente.")
	fmt.Printf("Panel de gestión:   http://localhost%s\n", cfg.Servidor.Direccion)
	fmt.Printf("Zona DNS:           %s (servidor %s)\n", cfg.DNS.Zona, cfg.DNS.Servidor)
	fmt.Printf("Disco plantilla:    %s\n", cfg.VirtualBox.DiscoPlantilla)
	fmt.Printf("Rango de IPs:       %s%d – %s%d\n", cfg.Red.Prefijo, cfg.Red.HostInicio, cfg.Red.Prefijo, cfg.Red.HostFin)
	fmt.Println("Presiona Ctrl+C para detener.")

	// 9. Esperar aquí hasta recibir Ctrl+C.
	<-ctx.Done()
	fmt.Println("\nDeteniendo el servidor...")

	// 10. Cerrar el servidor web: deja de aceptar peticiones y espera hasta
	//     10 s a que terminen las que estaban en curso.
	apagado, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if err := srv.Shutdown(apagado); err != nil {
		log.Printf("Cierre forzado del servidor: %v", err)
	}

	// 11. Esperar a que terminen las operaciones en segundo plano (crear,
	//     publicar, eliminar) para no dejar VMs a medio configurar.
	//     Un segundo Ctrl+C cierra de inmediato.
	fmt.Println("Esperando a que terminen las operaciones en curso (Ctrl+C otra vez para forzar)...")
	terminado := make(chan struct{}) // Canal que se cierra al terminar.
	go func() { aprov.Esperar(); close(terminado) }()
	forzar := make(chan os.Signal, 1) // Canal para un segundo Ctrl+C.
	signal.Notify(forzar, os.Interrupt, syscall.SIGTERM)
	select {
	case <-terminado: // Todo terminó normalmente.
	case <-forzar: // Se pidió cerrar de inmediato.
		fmt.Println("Cierre forzado: las operaciones pendientes quedarán marcadas como error.")
	}
	fmt.Println("Servidor detenido.")
}
