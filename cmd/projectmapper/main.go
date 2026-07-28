// Command projectmapper arranca el servidor local de Project Mapper.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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
			log.Fatalf("seed: %v", err)
		}
	}

	st, err := store.Load(*dataPath)
	if err != nil {
		log.Fatalf("cargar datos: %v", err)
	}

	srv := server.New(st)
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	url := "http://" + addr

	fmt.Printf("Project Mapper corriendo en %s — cierra esta ventana para detenerlo.\n", url)
	go openBrowser(url)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatalf("servidor: %v", err)
	}
}

// maybeSeed copia data/seed.json a dataPath si este último todavía no existe.
// Nunca sobreescribe datos existentes del usuario.
func maybeSeed(dataPath string) error {
	if _, err := os.Stat(dataPath); err == nil {
		return nil
	}
	b, err := os.ReadFile(filepath.Join("data", "seed.json"))
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
