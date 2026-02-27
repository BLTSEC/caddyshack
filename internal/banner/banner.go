package banner

import (
	"fmt"

	"github.com/BLTSEC/caddyshack/internal/config"
	"github.com/fatih/color"
)

// Print displays the ASCII banner, version, and disclaimer.
func Print() {
	cyan := color.New(color.FgCyan, color.Bold)
	yellow := color.New(color.FgYellow)

	cyan.Println(`
  ██████╗ █████╗ ██████╗ ██████╗ ██╗   ██╗███████╗██╗  ██╗ █████╗  ██████╗██╗  ██╗
 ██╔════╝██╔══██╗██╔══██╗██╔══██╗╚██╗ ██╔╝██╔════╝██║  ██║██╔══██╗██╔════╝██║ ██╔╝
 ██║     ███████║██║  ██║██║  ██║ ╚████╔╝ ███████╗███████║███████║██║     █████╔╝
 ██║     ██╔══██║██║  ██║██║  ██║  ╚██╔╝  ╚════██║██╔══██║██╔══██║██║     ██╔═██╗
 ╚██████╗██║  ██║██████╔╝██████╔╝   ██║   ███████║██║  ██║██║  ██║╚██████╗██║  ██╗
  ╚═════╝╚═╝  ╚═╝╚═════╝ ╚═════╝    ╚═╝   ╚══════╝╚═╝  ╚═╝╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝`)

	fmt.Printf("              Website Cloner & Credential Harvester v%s — by @BLTSEC\n\n", config.Version)
	yellow.Println("  [!] For authorized penetration testing only. Obtain written permission before use.")
	fmt.Println()
}
