// Command projectmapper arranca el servidor local de Project Mapper.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"projectmapper/internal/server"
	"projectmapper/internal/store"
)

func main() {
	port := flag.Int("port", 8787, "puerto de escucha (solo 127.0.0.1)")
	dataPath := flag.String("data", filepath.Join("data", "projects.json"), "ruta al archivo JSON de datos")
	seed := flag.Bool("seed", false, "si el archivo de datos no existe, inicializarlo con data/seed.json")
	flag.Parse()

	if *seed {
		if err := maybeSeed(*dataPath); err != nil {
			fatal("seed: %v", err)
		}
	}

	st, err := store.Load(*dataPath)
	if err != nil {
		fatal("cargar datos: %v", err)
	}

	srv := server.New(st)
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	url := "http://" + addr

	// Bind explícito antes de servir: si el puerto ya está ocupado por otra
	// instancia (segundo doble click en el atajo del escritorio) abrimos el
	// navegador en la que ya corre, en vez de morir con un error que nadie
	// llega a leer porque la consola se cierra al instante.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if alreadyRunning(url) {
			fmt.Printf("Project Mapper ya está corriendo en %s — abriendo el navegador.\n", url)
			openBrowser(url)
			return
		}
		fatal("no se pudo escuchar en %s: %v\nOtro programa está usando ese puerto; probá con -port <otro>.", addr, err)
	}

	fmt.Printf("Project Mapper corriendo en %s — cierra esta ventana para detenerlo.\n", url)
	go openBrowser(url)

	if err := http.Serve(ln, srv.Handler()); err != nil {
		fatal("servidor: %v", err)
	}
}

// maybeSeed copia data/seed.json a dataPath si este último todavía no existe.
// Nunca sobreescribe datos existentes del usuario.
func maybeSeed(dataPath string) error {
	if _, err := os.Stat(dataPath); err == nil {
		return nil
	}
	b, err := os.ReadFile(seedSource())
	if err != nil {
		return err
	}
	if dir := filepath.Dir(dataPath); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(dataPath, b, 0o644)
}

// seedSource resuelve la ruta de data/seed.json: primero relativa al
// directorio de trabajo (uso normal desde el repo) y, si ahí no está, junto al
// ejecutable. El atajo del escritorio fija WorkingDirectory al directorio de
// instalación, pero un doble click desde otra carpeta no.
func seedSource() string {
	rel := filepath.Join("data", "seed.json")
	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	exe, err := os.Executable()
	if err != nil {
		return rel
	}
	beside := filepath.Join(filepath.Dir(exe), "data", "seed.json")
	if _, err := os.Stat(beside); err == nil {
		return beside
	}
	return rel
}

// alreadyRunning distingue "el puerto lo tiene otra instancia de Project
// Mapper" de "el puerto lo tiene otro programa cualquiera": pide la home y
// busca la marca del layout en el HTML.
func alreadyRunning(url string) bool {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return false
	}
	return bytes.Contains(body, []byte("Project Mapper"))
}

// fatal reporta un fallo de arranque y espera un Enter antes de salir: cuando
// el programa se lanza desde el atajo, la ventana de consola desaparece al
// terminar el proceso, así que sin la pausa el error es invisible. Con stdin no
// interactivo (pipes, CI) el read devuelve EOF de inmediato y no cuelga nada.
func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	fmt.Fprint(os.Stderr, "Presiona Enter para cerrar...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	os.Exit(1)
}

// openBrowser abre la URL en el navegador default del sistema.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
