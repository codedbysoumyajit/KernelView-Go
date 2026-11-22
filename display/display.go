package display

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"

	"KernelView-Go/gather"
	"golang.org/x/term"
)

// Theme struct to hold color definitions (exported)
type Theme struct {
	Category string
	Key      string
	Value    string
	Accent   string
	Reset    string
}

// Define the standard themes (exported)
var (
	NormalTheme = Theme{
		Category: "\033[34m",       // Bright Blue
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[34m",       // Bright Blue
		Reset:    "\033[0m",
	}
	FastTheme = Theme{
		Category: "\033[36m",       // Bright Cyan
		Key:      "\033[38;5;255m", // White
		Value:    "\033[38;5;249m", // Light Gray
		Accent:   "\033[36m",       // Bright Cyan
		Reset:    "\033[0m",
	}
	BlankTheme = Theme{
		Category: "",
		Key:      "",
		Value:    "",
		Accent:   "",
		Reset:    "",
	}
)

var ansiRegexp = regexp.MustCompile("\x1b\\[[0-9;?]*[a-zA-Z]")

// --- Internal Helper Functions ---

func stripAnsi(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

func truncateString(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) > maxLen {
		if maxLen < 1 {
			maxLen = 1
		}
		return string(runes[:maxLen-1]) + "…" // Use ellipsis character
	}
	return s
}

// Get terminal width helper
func getTerminalWidth() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 {
		return 80 // Default width
	}
	return width
}

func getUsername() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "user"
}

func getDistroKey() string {
	if runtime.GOOS == "windows" {
		return "windows"
	}
	if runtime.GOOS == "darwin" {
		return "macos"
	}
	if runtime.GOOS == "freebsd" {
		return "freebsd"
	}
	if runtime.GOOS == "openbsd" {
		return "openbsd"
	}
	if runtime.GOOS == "netbsd" {
		return "netbsd"
	}
	if runtime.GOOS == "dragonfly" {
		return "dragonfly"
	}
	if runtime.GOOS == "solaris" || runtime.GOOS == "illumos" {
		return "solaris"
	}
	if runtime.GOOS == "android" {
		return "android"
	}

	// Read standard /etc/os-release or /usr/lib/os-release (with PREFIX prioritized for chroot/Termux)
	var osReleasePaths []string
	if prefix := os.Getenv("PREFIX"); prefix != "" {
		osReleasePaths = append(osReleasePaths, prefix+"/etc/os-release")
	}
	osReleasePaths = append(osReleasePaths, "/etc/os-release", "/usr/lib/os-release")

	for _, path := range osReleasePaths {
		if content, err := os.ReadFile(path); err == nil {
			contentStr := strings.ToLower(string(content))
			lines := strings.Split(contentStr, "\n")
			var id, idLike string
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "id=") {
					id = strings.Trim(strings.TrimPrefix(line, "id="), "\"'\r ")
				}
				if strings.HasPrefix(line, "id_like=") {
					idLike = strings.Trim(strings.TrimPrefix(line, "id_like="), "\"'\r ")
				}
			}

			// Exact ID match first
			switch id {
			case "generic", "unknown":
				return "generic"
			case "manjaro":
				return "manjaro"
			case "arch":
				return "arch"
			case "endeavouros":
				return "endeavouros"
			case "garuda":
				return "garuda"
			case "cachyos":
				return "cachyos"
			case "bazzite":
				return "bazzite"
			case "steamos":
				return "steamos"
			case "nobara":
				return "nobara"
			case "kali":
				return "kali"
			case "parrot":
				return "parrot"
			case "zorin":
				return "zorin"
			case "mx", "mx-linux":
				return "mx"
			case "neon", "kdeneon":
				return "neon"
			case "elementary":
				return "elementary"
			case "deepin":
				return "deepin"
			case "tails":
				return "tails"
			case "qubes":
				return "qubes"
			case "rocky", "rockylinux":
				return "rocky"
			case "almalinux", "alma":
				return "almalinux"
			case "ol", "oracle":
				return "oracle"
			case "solus":
				return "solus"
			case "puppy":
				return "puppy"
			case "peppermint":
				return "peppermint"
			case "antix":
				return "antix"
			case "clear-linux-os", "clearlinux", "clear":
				return "clearlinux"
			case "pclinuxos":
				return "pclinuxos"
			case "pop":
				return "pop"
			case "nixos":
				return "nixos"
			case "void":
				return "void"
			case "ubuntu":
				return "ubuntu"
			case "debian":
				return "debian"
			case "fedora":
				return "fedora"
			case "centos":
				return "centos"
			case "rhel", "redhat":
				return "redhat"
			case "alpine":
				return "alpine"
			case "gentoo":
				return "gentoo"
			case "opensuse", "suse", "opensuse-tumbleweed", "opensuse-leap":
				return "opensuse"
			case "linuxmint", "mint":
				return "mint"
			case "slackware":
				return "slackware"
			case "raspbian":
				return "raspbian"
			case "artix":
				return "artix"
			case "devuan":
				return "devuan"
			case "mageia":
				return "mageia"
			case "arcolinux", "arco":
				return "arcolinux"
			case "kaos":
				return "kaos"
			case "guix":
				return "guix"
			case "bodhi":
				return "bodhi"
			case "asahi":
				return "asahi"
			case "dragonfly":
				return "dragonfly"
			case "solaris", "illumos", "openindiana":
				return "solaris"
			case "haiku":
				return "haiku"
			case "android":
				return "android"
			case "chromeos", "chromiumos":
				return "chromeos"
			case "archcraft":
				return "archcraft"
			case "blackarch":
				return "blackarch"
			case "athena", "athenaos":
				return "athena"
			case "endless":
				return "endless"
			case "bunsenlabs":
				return "bunsenlabs"
			case "pureos":
				return "pureos"
			case "crux":
				return "crux"
			case "chimera", "chimeraphile":
				return "chimera"
			case "vanilla", "vanillaos":
				return "vanilla"
			case "aurora":
				return "aurora"
			case "postmarketos":
				return "postmarketos"
			case "kubuntu":
				return "kubuntu"
			case "xubuntu":
				return "xubuntu"
			case "lubuntu":
				return "lubuntu"
			case "openwrt":
				return "openwrt"
			case "rebornos":
				return "rebornos"
			case "trisquel":
				return "trisquel"
			case "hyperbola":
				return "hyperbola"
			case "parabola":
				return "parabola"
			case "minix":
				return "minix"
			case "amazon", "amzn":
				return "amazon"
			case "kdelinux", "kde":
				return "kdelinux"
			case "pve", "proxmox":
				return "proxmox"
			case "armbian":
				return "armbian"
			case "dietpi":
				return "dietpi"
			case "linuxlite", "lite":
				return "linuxlite"
			case "ghostbsd":
				return "ghostbsd"
			case "nomadbsd":
				return "nomadbsd"
			case "midnightbsd":
				return "midnightbsd"
			case "archlabs":
				return "archlabs"
			case "antergos":
				return "antergos"
			case "siduction":
				return "siduction"
			case "altlinux", "alt":
				return "altlinux"
			case "openmandriva":
				return "openmandriva"
			case "rosa":
				return "rosa"
			case "redstar":
				return "redstar"
			case "scientific", "scientificlinux", "sl":
				return "scientific"
			case "feren", "ferenos":
				return "feren"
			case "nitrux":
				return "nitrux"
			case "sparky", "sparkylinux":
				return "sparky"
			case "coreos", "fedora-coreos":
				return "coreos"
			case "flatcar":
				return "flatcar"
			case "openeuler":
				return "openeuler"
			case "kylin", "openkylin":
				return "kylin"
			case "uos":
				return "uos"
			case "smartos":
				return "smartos"
			case "bedrock":
				return "bedrock"
			case "chakra":
				return "chakra"
			case "netrunner":
				return "netrunner"
			case "exherbo":
				return "exherbo"
			case "slitaz":
				return "slitaz"
			case "frugalware":
				return "frugalware"
			}

			// Substring / id_like fallback
			combined := id + " " + idLike + " " + contentStr
			if strings.Contains(combined, "manjaro") {
				return "manjaro"
			}
			if strings.Contains(combined, "endeavouros") || strings.Contains(combined, "endeavour") {
				return "endeavouros"
			}
			if strings.Contains(combined, "garuda") {
				return "garuda"
			}
			if strings.Contains(combined, "cachyos") {
				return "cachyos"
			}
			if strings.Contains(combined, "bazzite") {
				return "bazzite"
			}
			if strings.Contains(combined, "steamos") || strings.Contains(combined, "steam") {
				return "steamos"
			}
			if strings.Contains(combined, "nobara") {
				return "nobara"
			}
			if strings.Contains(combined, "kali") {
				return "kali"
			}
			if strings.Contains(combined, "parrot") {
				return "parrot"
			}
			if strings.Contains(combined, "zorin") {
				return "zorin"
			}
			if strings.Contains(combined, "mx linux") || strings.Contains(combined, "mx-linux") || strings.Contains(combined, "mx") {
				return "mx"
			}
			if strings.Contains(combined, "kde neon") || strings.Contains(combined, "kdeneon") || strings.Contains(combined, "neon") {
				return "neon"
			}
			if strings.Contains(combined, "elementary") {
				return "elementary"
			}
			if strings.Contains(combined, "deepin") {
				return "deepin"
			}
			if strings.Contains(combined, "tails") {
				return "tails"
			}
			if strings.Contains(combined, "qubes") {
				return "qubes"
			}
			if strings.Contains(combined, "rocky") {
				return "rocky"
			}
			if strings.Contains(combined, "almalinux") || strings.Contains(combined, "alma") {
				return "almalinux"
			}
			if strings.Contains(combined, "oracle") {
				return "oracle"
			}
			if strings.Contains(combined, "solus") {
				return "solus"
			}
			if strings.Contains(combined, "puppy") {
				return "puppy"
			}
			if strings.Contains(combined, "peppermint") {
				return "peppermint"
			}
			if strings.Contains(combined, "antix") {
				return "antix"
			}
			if strings.Contains(combined, "clear-linux") || strings.Contains(combined, "clearlinux") {
				return "clearlinux"
			}
			if strings.Contains(combined, "pclinuxos") {
				return "pclinuxos"
			}
			if strings.Contains(combined, "pop!_os") || strings.Contains(combined, "pop-os") || strings.Contains(combined, "pop_os") {
				return "pop"
			}
			if strings.Contains(combined, "nixos") {
				return "nixos"
			}
			if strings.Contains(combined, "void") {
				return "void"
			}
			if strings.Contains(combined, "arch") {
				return "arch"
			}
			if strings.Contains(combined, "ubuntu") {
				return "ubuntu"
			}
			if strings.Contains(combined, "debian") {
				return "debian"
			}
			if strings.Contains(combined, "fedora") {
				return "fedora"
			}
			if strings.Contains(combined, "centos") {
				return "centos"
			}
			if strings.Contains(combined, "redhat") || strings.Contains(combined, "red hat") || strings.Contains(combined, "rhel") {
				return "redhat"
			}
			if strings.Contains(combined, "alpine") {
				return "alpine"
			}
			if strings.Contains(combined, "gentoo") {
				return "gentoo"
			}
			if strings.Contains(combined, "suse") || strings.Contains(combined, "opensuse") {
				return "opensuse"
			}
			if strings.Contains(combined, "mint") {
				return "mint"
			}
			if strings.Contains(combined, "slackware") {
				return "slackware"
			}
			if strings.Contains(combined, "raspbian") || strings.Contains(combined, "raspberry") {
				return "raspbian"
			}
			if strings.Contains(combined, "artix") {
				return "artix"
			}
			if strings.Contains(combined, "devuan") {
				return "devuan"
			}
			if strings.Contains(combined, "mageia") {
				return "mageia"
			}
			if strings.Contains(combined, "arcolinux") || strings.Contains(combined, "arco") {
				return "arcolinux"
			}
			if strings.Contains(combined, "kaos") {
				return "kaos"
			}
			if strings.Contains(combined, "guix") {
				return "guix"
			}
			if strings.Contains(combined, "bodhi") {
				return "bodhi"
			}
			if strings.Contains(combined, "asahi") {
				return "asahi"
			}
			if strings.Contains(combined, "dragonfly") {
				return "dragonfly"
			}
			if strings.Contains(combined, "solaris") || strings.Contains(combined, "illumos") || strings.Contains(combined, "openindiana") {
				return "solaris"
			}
			if strings.Contains(combined, "haiku") {
				return "haiku"
			}
			if strings.Contains(combined, "android") {
				return "android"
			}
			if strings.Contains(combined, "chromeos") || strings.Contains(combined, "chromium") {
				return "chromeos"
			}
			if strings.Contains(combined, "archcraft") {
				return "archcraft"
			}
			if strings.Contains(combined, "blackarch") {
				return "blackarch"
			}
			if strings.Contains(combined, "athena") {
				return "athena"
			}
			if strings.Contains(combined, "endless") {
				return "endless"
			}
			if strings.Contains(combined, "bunsenlabs") {
				return "bunsenlabs"
			}
			if strings.Contains(combined, "pureos") {
				return "pureos"
			}
			if strings.Contains(combined, "crux") {
				return "crux"
			}
			if strings.Contains(combined, "chimera") {
				return "chimera"
			}
			if strings.Contains(combined, "vanilla") {
				return "vanilla"
			}
			if strings.Contains(combined, "aurora") {
				return "aurora"
			}
			if strings.Contains(combined, "postmarketos") {
				return "postmarketos"
			}
			if strings.Contains(combined, "kubuntu") {
				return "kubuntu"
			}
			if strings.Contains(combined, "xubuntu") {
				return "xubuntu"
			}
			if strings.Contains(combined, "lubuntu") {
				return "lubuntu"
			}
			if strings.Contains(combined, "openwrt") {
				return "openwrt"
			}
			if strings.Contains(combined, "rebornos") {
				return "rebornos"
			}
			if strings.Contains(combined, "trisquel") {
				return "trisquel"
			}
			if strings.Contains(combined, "hyperbola") {
				return "hyperbola"
			}
			if strings.Contains(combined, "parabola") {
				return "parabola"
			}
			if strings.Contains(combined, "minix") {
				return "minix"
			}
			if strings.Contains(combined, "amazon") || strings.Contains(combined, "amzn") {
				return "amazon"
			}
			if strings.Contains(combined, "kdelinux") || strings.Contains(combined, "kde linux") {
				return "kdelinux"
			}
			if strings.Contains(combined, "proxmox") || strings.Contains(combined, "pve") {
				return "proxmox"
			}
			if strings.Contains(combined, "armbian") {
				return "armbian"
			}
			if strings.Contains(combined, "dietpi") {
				return "dietpi"
			}
			if strings.Contains(combined, "linuxlite") || strings.Contains(combined, "linux lite") {
				return "linuxlite"
			}
			if strings.Contains(combined, "ghostbsd") {
				return "ghostbsd"
			}
			if strings.Contains(combined, "nomadbsd") {
				return "nomadbsd"
			}
			if strings.Contains(combined, "midnightbsd") {
				return "midnightbsd"
			}
			if strings.Contains(combined, "archlabs") {
				return "archlabs"
			}
			if strings.Contains(combined, "antergos") {
				return "antergos"
			}
			if strings.Contains(combined, "siduction") {
				return "siduction"
			}
			if strings.Contains(combined, "altlinux") || strings.Contains(combined, "alt linux") {
				return "altlinux"
			}
			if strings.Contains(combined, "openmandriva") {
				return "openmandriva"
			}
			if strings.Contains(combined, "rosa") {
				return "rosa"
			}
			if strings.Contains(combined, "redstar") || strings.Contains(combined, "red star") {
				return "redstar"
			}
			if strings.Contains(combined, "scientific") {
				return "scientific"
			}
			if strings.Contains(combined, "feren") {
				return "feren"
			}
			if strings.Contains(combined, "nitrux") {
				return "nitrux"
			}
			if strings.Contains(combined, "sparky") {
				return "sparky"
			}
			if strings.Contains(combined, "coreos") {
				return "coreos"
			}
			if strings.Contains(combined, "flatcar") {
				return "flatcar"
			}
			if strings.Contains(combined, "openeuler") {
				return "openeuler"
			}
			if strings.Contains(combined, "kylin") {
				return "kylin"
			}
			if strings.Contains(combined, "uos") || strings.Contains(combined, "uniontech") {
				return "uos"
			}
			if strings.Contains(combined, "smartos") {
				return "smartos"
			}
			if strings.Contains(combined, "bedrock") {
				return "bedrock"
			}
			if strings.Contains(combined, "chakra") {
				return "chakra"
			}
			if strings.Contains(combined, "netrunner") {
				return "netrunner"
			}
			if strings.Contains(combined, "exherbo") {
				return "exherbo"
			}
			if strings.Contains(combined, "slitaz") {
				return "slitaz"
			}
			if strings.Contains(combined, "frugalware") {
				return "frugalware"
			}
		}
	}
	if runtime.GOOS == "linux" {
		return "linux"
	}
	return "generic"
}

func GetThemeForDistro(distroKey string, noColor bool) Theme {
	if noColor {
		return BlankTheme
	}

	valColor := "\033[38;5;253m" // Light gray for values
	reset := "\033[0m"

	switch distroKey {
	case "amazon":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Amazon Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "kdelinux":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold KDE Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "proxmox":
		return Theme{
			Category: "\033[1;38;5;202m", // Bold Proxmox Orange
			Key:      "\033[1;38;5;202m",
			Value:    valColor,
			Accent:   "\033[1;38;5;202m",
			Reset:    reset,
		}
	case "armbian":
		return Theme{
			Category: "\033[1;38;5;196m", // Bold Armbian Red
			Key:      "\033[1;38;5;196m",
			Value:    valColor,
			Accent:   "\033[1;38;5;196m",
			Reset:    reset,
		}
	case "dietpi":
		return Theme{
			Category: "\033[1;38;5;76m", // Bold DietPi Green
			Key:      "\033[1;38;5;76m",
			Value:    valColor,
			Accent:   "\033[1;38;5;76m",
			Reset:    reset,
		}
	case "linuxlite":
		return Theme{
			Category: "\033[1;38;5;148m", // Bold Linux Lite Lime
			Key:      "\033[1;38;5;148m",
			Value:    valColor,
			Accent:   "\033[1;38;5;148m",
			Reset:    reset,
		}
	case "ghostbsd":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold GhostBSD Cyan
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "nomadbsd":
		return Theme{
			Category: "\033[1;38;5;67m", // Bold NomadBSD Steel Blue
			Key:      "\033[1;38;5;67m",
			Value:    valColor,
			Accent:   "\033[1;38;5;67m",
			Reset:    reset,
		}
	case "midnightbsd":
		return Theme{
			Category: "\033[1;38;5;97m", // Bold MidnightBSD Violet
			Key:      "\033[1;38;5;97m",
			Value:    valColor,
			Accent:   "\033[1;38;5;97m",
			Reset:    reset,
		}
	case "archlabs":
		return Theme{
			Category: "\033[1;38;5;66m", // Bold ArchLabs Slate
			Key:      "\033[1;38;5;66m",
			Value:    valColor,
			Accent:   "\033[1;38;5;66m",
			Reset:    reset,
		}
	case "antergos":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Antergos Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "siduction":
		return Theme{
			Category: "\033[1;38;5;32m", // Bold Siduction Blue
			Key:      "\033[1;38;5;32m",
			Value:    valColor,
			Accent:   "\033[1;38;5;32m",
			Reset:    reset,
		}
	case "altlinux":
		return Theme{
			Category: "\033[1;38;5;214m", // Bold ALT Linux Gold
			Key:      "\033[1;38;5;214m",
			Value:    valColor,
			Accent:   "\033[1;38;5;214m",
			Reset:    reset,
		}
	case "openmandriva":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold OpenMandriva Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "rosa":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold ROSA Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "redstar":
		return Theme{
			Category: "\033[1;31m", // Bold Red Star Crimson
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "scientific":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Scientific Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "feren":
		return Theme{
			Category: "\033[1;38;5;38m", // Bold Feren Sky Blue
			Key:      "\033[1;38;5;38m",
			Value:    valColor,
			Accent:   "\033[1;38;5;38m",
			Reset:    reset,
		}
	case "nitrux":
		return Theme{
			Category: "\033[1;38;5;176m", // Bold Nitrux Purple
			Key:      "\033[1;38;5;176m",
			Value:    valColor,
			Accent:   "\033[1;38;5;176m",
			Reset:    reset,
		}
	case "sparky":
		return Theme{
			Category: "\033[1;38;5;196m", // Bold Sparky Red
			Key:      "\033[1;38;5;196m",
			Value:    valColor,
			Accent:   "\033[1;38;5;196m",
			Reset:    reset,
		}
	case "coreos":
		return Theme{
			Category: "\033[1;38;5;32m", // Bold CoreOS Blue
			Key:      "\033[1;38;5;32m",
			Value:    valColor,
			Accent:   "\033[1;38;5;32m",
			Reset:    reset,
		}
	case "flatcar":
		return Theme{
			Category: "\033[1;38;5;37m", // Bold Flatcar Aqua
			Key:      "\033[1;38;5;37m",
			Value:    valColor,
			Accent:   "\033[1;38;5;37m",
			Reset:    reset,
		}
	case "openeuler":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold openEuler Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "kylin":
		return Theme{
			Category: "\033[1;38;5;27m", // Bold Kylin Blue
			Key:      "\033[1;38;5;27m",
			Value:    valColor,
			Accent:   "\033[1;38;5;27m",
			Reset:    reset,
		}
	case "uos":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold UOS Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "smartos":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold SmartOS Azure
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "bedrock":
		return Theme{
			Category: "\033[1;37m", // Bold Bedrock White
			Key:      "\033[1;37m",
			Value:    valColor,
			Accent:   "\033[1;37m",
			Reset:    reset,
		}
	case "chakra":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Chakra Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "netrunner":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Netrunner Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "exherbo":
		return Theme{
			Category: "\033[1;38;5;67m", // Bold Exherbo Steel
			Key:      "\033[1;38;5;67m",
			Value:    valColor,
			Accent:   "\033[1;38;5;67m",
			Reset:    reset,
		}
	case "slitaz":
		return Theme{
			Category: "\033[1;38;5;214m", // Bold SliTaz Amber
			Key:      "\033[1;38;5;214m",
			Value:    valColor,
			Accent:   "\033[1;38;5;214m",
			Reset:    reset,
		}
	case "frugalware":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Frugalware Azure
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "slackware":
		return Theme{
			Category: "\033[1;34m", // Bold Slackware Blue
			Key:      "\033[1;34m",
			Value:    valColor,
			Accent:   "\033[1;34m",
			Reset:    reset,
		}
	case "raspbian":
		return Theme{
			Category: "\033[1;38;5;197m", // Bold Raspberry Red
			Key:      "\033[1;38;5;197m",
			Value:    valColor,
			Accent:   "\033[1;38;5;197m",
			Reset:    reset,
		}
	case "artix":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Artix Cyan
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "devuan":
		return Theme{
			Category: "\033[1;38;5;134m", // Bold Devuan Violet
			Key:      "\033[1;38;5;134m",
			Value:    valColor,
			Accent:   "\033[1;38;5;134m",
			Reset:    reset,
		}
	case "mageia":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Mageia Cyan
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "arcolinux":
		return Theme{
			Category: "\033[1;38;5;32m", // Bold Arco Blue
			Key:      "\033[1;38;5;32m",
			Value:    valColor,
			Accent:   "\033[1;38;5;32m",
			Reset:    reset,
		}
	case "kaos":
		return Theme{
			Category: "\033[1;38;5;67m", // Bold KaOS Steel Blue
			Key:      "\033[1;38;5;67m",
			Value:    valColor,
			Accent:   "\033[1;38;5;67m",
			Reset:    reset,
		}
	case "guix":
		return Theme{
			Category: "\033[1;38;5;214m", // Bold Guix Gold
			Key:      "\033[1;38;5;214m",
			Value:    valColor,
			Accent:   "\033[1;38;5;214m",
			Reset:    reset,
		}
	case "bodhi":
		return Theme{
			Category: "\033[1;38;5;76m", // Bold Bodhi Green
			Key:      "\033[1;38;5;76m",
			Value:    valColor,
			Accent:   "\033[1;38;5;76m",
			Reset:    reset,
		}
	case "asahi":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Asahi Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "dragonfly":
		return Theme{
			Category: "\033[1;31m", // Bold DragonFly Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "solaris":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Solaris Solar Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "haiku":
		return Theme{
			Category: "\033[1;38;5;214m", // Bold Haiku Amber
			Key:      "\033[1;38;5;214m",
			Value:    valColor,
			Accent:   "\033[1;38;5;214m",
			Reset:    reset,
		}
	case "android":
		return Theme{
			Category: "\033[1;38;5;119m", // Bold Android Lime Green
			Key:      "\033[1;38;5;119m",
			Value:    valColor,
			Accent:   "\033[1;38;5;119m",
			Reset:    reset,
		}
	case "chromeos":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Chrome Sky Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "archcraft":
		return Theme{
			Category: "\033[1;38;5;116m", // Bold Archcraft Pastel Cyan
			Key:      "\033[1;38;5;116m",
			Value:    valColor,
			Accent:   "\033[1;38;5;116m",
			Reset:    reset,
		}
	case "blackarch":
		return Theme{
			Category: "\033[1;31m", // Bold BlackArch Crimson
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "athena":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Athena Cyber Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "endless":
		return Theme{
			Category: "\033[1;38;5;203m", // Bold Endless Coral Red
			Key:      "\033[1;38;5;203m",
			Value:    valColor,
			Accent:   "\033[1;38;5;203m",
			Reset:    reset,
		}
	case "bunsenlabs":
		return Theme{
			Category: "\033[1;37m", // Bold BunsenLabs Silver
			Key:      "\033[1;37m",
			Value:    valColor,
			Accent:   "\033[1;37m",
			Reset:    reset,
		}
	case "pureos":
		return Theme{
			Category: "\033[1;38;5;35m", // Bold PureOS Emerald Green
			Key:      "\033[1;38;5;35m",
			Value:    valColor,
			Accent:   "\033[1;38;5;35m",
			Reset:    reset,
		}
	case "crux":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold CRUX Ice Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "chimera":
		return Theme{
			Category: "\033[1;38;5;197m", // Bold Chimera Crimson
			Key:      "\033[1;38;5;197m",
			Value:    valColor,
			Accent:   "\033[1;38;5;197m",
			Reset:    reset,
		}
	case "vanilla":
		return Theme{
			Category: "\033[1;38;5;221m", // Bold Vanilla Gold
			Key:      "\033[1;38;5;221m",
			Value:    valColor,
			Accent:   "\033[1;38;5;221m",
			Reset:    reset,
		}
	case "aurora":
		return Theme{
			Category: "\033[1;38;5;135m", // Bold Aurora Violet
			Key:      "\033[1;38;5;135m",
			Value:    valColor,
			Accent:   "\033[1;38;5;135m",
			Reset:    reset,
		}
	case "postmarketos":
		return Theme{
			Category: "\033[1;38;5;29m", // Bold postmarketOS Pine Green
			Key:      "\033[1;38;5;29m",
			Value:    valColor,
			Accent:   "\033[1;38;5;29m",
			Reset:    reset,
		}
	case "kubuntu":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Kubuntu Ocean Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "xubuntu":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Xubuntu Sky Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "lubuntu":
		return Theme{
			Category: "\033[1;38;5;32m", // Bold Lubuntu Light Blue
			Key:      "\033[1;38;5;32m",
			Value:    valColor,
			Accent:   "\033[1;38;5;32m",
			Reset:    reset,
		}
	case "openwrt":
		return Theme{
			Category: "\033[1;38;5;37m", // Bold OpenWrt Turquoise
			Key:      "\033[1;38;5;37m",
			Value:    valColor,
			Accent:   "\033[1;38;5;37m",
			Reset:    reset,
		}
	case "rebornos":
		return Theme{
			Category: "\033[1;38;5;26m", // Bold RebornOS Deep Blue
			Key:      "\033[1;38;5;26m",
			Value:    valColor,
			Accent:   "\033[1;38;5;26m",
			Reset:    reset,
		}
	case "trisquel":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Trisquel Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "hyperbola":
		return Theme{
			Category: "\033[1;37m", // Bold Hyperbola Pure White
			Key:      "\033[1;37m",
			Value:    valColor,
			Accent:   "\033[1;37m",
			Reset:    reset,
		}
	case "parabola":
		return Theme{
			Category: "\033[1;38;5;129m", // Bold Parabola Purple
			Key:      "\033[1;38;5;129m",
			Value:    valColor,
			Accent:   "\033[1;38;5;129m",
			Reset:    reset,
		}
	case "minix":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Minix Sky Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "manjaro":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "arch":
		return Theme{
			Category: "\033[1;36m", // Bold Cyan
			Key:      "\033[1;36m",
			Value:    valColor,
			Accent:   "\033[1;36m",
			Reset:    reset,
		}
	case "endeavouros":
		return Theme{
			Category: "\033[1;38;5;127m", // Bold Endeavour Purple
			Key:      "\033[1;38;5;127m",
			Value:    valColor,
			Accent:   "\033[1;38;5;127m",
			Reset:    reset,
		}
	case "garuda":
		return Theme{
			Category: "\033[1;38;5;202m", // Bold Garuda Orange-Red
			Key:      "\033[1;38;5;202m",
			Value:    valColor,
			Accent:   "\033[1;38;5;202m",
			Reset:    reset,
		}
	case "cachyos":
		return Theme{
			Category: "\033[1;38;5;35m", // Bold CachyOS Emerald Green
			Key:      "\033[1;38;5;35m",
			Value:    valColor,
			Accent:   "\033[1;38;5;35m",
			Reset:    reset,
		}
	case "bazzite":
		return Theme{
			Category: "\033[1;38;5;135m", // Bold Bazzite Purple
			Key:      "\033[1;38;5;135m",
			Value:    valColor,
			Accent:   "\033[1;38;5;135m",
			Reset:    reset,
		}
	case "steamos":
		return Theme{
			Category: "\033[1;34m", // Bold Steam Blue
			Key:      "\033[1;34m",
			Value:    valColor,
			Accent:   "\033[1;34m",
			Reset:    reset,
		}
	case "nobara":
		return Theme{
			Category: "\033[1;38;5;141m", // Bold Nobara Purple
			Key:      "\033[1;38;5;141m",
			Value:    valColor,
			Accent:   "\033[1;38;5;141m",
			Reset:    reset,
		}
	case "kali":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Kali Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "parrot":
		return Theme{
			Category: "\033[1;38;5;44m", // Bold Parrot Turquoise
			Key:      "\033[1;38;5;44m",
			Value:    valColor,
			Accent:   "\033[1;38;5;44m",
			Reset:    reset,
		}
	case "zorin":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Zorin Light Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "mx":
		return Theme{
			Category: "\033[1;37m", // Bold MX Gray/White
			Key:      "\033[1;37m",
			Value:    valColor,
			Accent:   "\033[1;37m",
			Reset:    reset,
		}
	case "neon":
		return Theme{
			Category: "\033[1;38;5;45m", // Bold KDE Neon Bright Cyan
			Key:      "\033[1;38;5;45m",
			Value:    valColor,
			Accent:   "\033[1;38;5;45m",
			Reset:    reset,
		}
	case "elementary":
		return Theme{
			Category: "\033[1;38;5;69m", // Bold elementary Sky Blue
			Key:      "\033[1;38;5;69m",
			Value:    valColor,
			Accent:   "\033[1;38;5;69m",
			Reset:    reset,
		}
	case "deepin":
		return Theme{
			Category: "\033[1;38;5;38m", // Bold Deepin Cyan
			Key:      "\033[1;38;5;38m",
			Value:    valColor,
			Accent:   "\033[1;38;5;38m",
			Reset:    reset,
		}
	case "tails":
		return Theme{
			Category: "\033[1;38;5;97m", // Bold Tails Purple
			Key:      "\033[1;38;5;97m",
			Value:    valColor,
			Accent:   "\033[1;38;5;97m",
			Reset:    reset,
		}
	case "qubes":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Qubes Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "rocky":
		return Theme{
			Category: "\033[1;38;5;34m", // Bold Rocky Forest Green
			Key:      "\033[1;38;5;34m",
			Value:    valColor,
			Accent:   "\033[1;38;5;34m",
			Reset:    reset,
		}
	case "almalinux":
		return Theme{
			Category: "\033[1;38;5;214m", // Bold AlmaLinux Amber
			Key:      "\033[1;38;5;214m",
			Value:    valColor,
			Accent:   "\033[1;38;5;214m",
			Reset:    reset,
		}
	case "oracle":
		return Theme{
			Category: "\033[1;31m", // Bold Oracle Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "solus":
		return Theme{
			Category: "\033[1;38;5;67m", // Bold Solus Slate Blue
			Key:      "\033[1;38;5;67m",
			Value:    valColor,
			Accent:   "\033[1;38;5;67m",
			Reset:    reset,
		}
	case "puppy":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold Puppy Light Steel Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "peppermint":
		return Theme{
			Category: "\033[1;38;5;160m", // Bold Peppermint Crimson
			Key:      "\033[1;38;5;160m",
			Value:    valColor,
			Accent:   "\033[1;38;5;160m",
			Reset:    reset,
		}
	case "antix":
		return Theme{
			Category: "\033[1;31m", // Bold antiX Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "clearlinux":
		return Theme{
			Category: "\033[1;38;5;39m", // Bold Clear Linux Sky Blue
			Key:      "\033[1;38;5;39m",
			Value:    valColor,
			Accent:   "\033[1;38;5;39m",
			Reset:    reset,
		}
	case "pclinuxos":
		return Theme{
			Category: "\033[1;34m", // Bold PCLinuxOS Blue
			Key:      "\033[1;34m",
			Value:    valColor,
			Accent:   "\033[1;34m",
			Reset:    reset,
		}
	case "ubuntu":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "pop":
		return Theme{
			Category: "\033[1;36m", // Bold Cyan / Teal
			Key:      "\033[1;36m",
			Value:    valColor,
			Accent:   "\033[1;36m",
			Reset:    reset,
		}
	case "nixos":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold Soft Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "void":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "debian":
		return Theme{
			Category: "\033[1;38;5;161m", // Bold Crimson
			Key:      "\033[1;38;5;161m",
			Value:    valColor,
			Accent:   "\033[1;38;5;161m",
			Reset:    reset,
		}
	case "fedora":
		return Theme{
			Category: "\033[1;38;5;75m", // Bold Soft Blue
			Key:      "\033[1;38;5;75m",
			Value:    valColor,
			Accent:   "\033[1;38;5;75m",
			Reset:    reset,
		}
	case "macos":
		return Theme{
			Category: "\033[1;35m", // Bold Purple
			Key:      "\033[1;35m",
			Value:    valColor,
			Accent:   "\033[1;35m",
			Reset:    reset,
		}
	case "windows":
		return Theme{
			Category: "\033[1;38;5;33m", // Bold Sky Blue
			Key:      "\033[1;38;5;33m",
			Value:    valColor,
			Accent:   "\033[1;38;5;33m",
			Reset:    reset,
		}
	case "freebsd":
		return Theme{
			Category: "\033[1;31m", // Bold Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "openbsd":
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	case "netbsd":
		return Theme{
			Category: "\033[1;38;5;208m", // Bold Orange
			Key:      "\033[1;38;5;208m",
			Value:    valColor,
			Accent:   "\033[1;38;5;208m",
			Reset:    reset,
		}
	case "alpine":
		return Theme{
			Category: "\033[1;34m", // Bold Blue
			Key:      "\033[1;34m",
			Value:    valColor,
			Accent:   "\033[1;34m",
			Reset:    reset,
		}
	case "gentoo":
		return Theme{
			Category: "\033[1;35m", // Bold Purple/Magenta
			Key:      "\033[1;35m",
			Value:    valColor,
			Accent:   "\033[1;35m",
			Reset:    reset,
		}
	case "redhat":
		return Theme{
			Category: "\033[1;31m", // Bold Red
			Key:      "\033[1;31m",
			Value:    valColor,
			Accent:   "\033[1;31m",
			Reset:    reset,
		}
	case "opensuse":
		return Theme{
			Category: "\033[1;32m", // Bold Green
			Key:      "\033[1;32m",
			Value:    valColor,
			Accent:   "\033[1;32m",
			Reset:    reset,
		}
	case "mint":
		return Theme{
			Category: "\033[1;38;5;120m", // Bold Mint Green
			Key:      "\033[1;38;5;120m",
			Value:    valColor,
			Accent:   "\033[1;38;5;120m",
			Reset:    reset,
		}
	case "centos":
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	case "generic", "unknown":
		return Theme{
			Category: "\033[1;36m", // Bold Cyan
			Key:      "\033[1;36m",
			Value:    valColor,
			Accent:   "\033[1;36m",
			Reset:    reset,
		}
	default: // linux fallback
		return Theme{
			Category: "\033[1;33m", // Bold Yellow
			Key:      "\033[1;33m",
			Value:    valColor,
			Accent:   "\033[1;33m",
			Reset:    reset,
		}
	}
}

func getColorBlocks(theme Theme) string {
	if theme.Reset == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("  ")
	for i := 0; i < 8; i++ {
		sb.WriteString(fmt.Sprintf("\033[3%dm███", i))
	}
	sb.WriteString("\033[0m\n  ")
	for i := 0; i < 8; i++ {
		sb.WriteString(fmt.Sprintf("\033[9%dm███", i))
	}
	sb.WriteString("\033[0m")
	return sb.String()
}

// DisplayLayout prints the logo and info side-by-side.
func DisplayLayout(logoKey string, infoLines []string, theme Theme) {
	logoLines, exists := DistroLogos[logoKey]
	if !exists {
		if runtime.GOOS == "linux" {
			logoLines = DistroLogos["linux"]
		} else {
			logoLines = DistroLogos["generic"]
		}
	}

	coloredLogoLines := make([]string, len(logoLines))
	for i, line := range logoLines {
		if theme.Accent != "" {
			coloredLogoLines[i] = theme.Accent + line + theme.Reset
		} else {
			coloredLogoLines[i] = line
		}
	}

	maxLines := len(coloredLogoLines)
	if len(infoLines) > maxLines {
		maxLines = len(infoLines)
	}

	logoWidth := 0
	for _, line := range logoLines {
		runesCount := utf8.RuneCountInString(line)
		if runesCount > logoWidth {
			logoWidth = runesCount
		}
	}

	termWidth := getTerminalWidth()

	// If the terminal is too narrow, fall back to pure text rendering without logo
	if termWidth < logoWidth+25 {
		for _, line := range infoLines {
			fmt.Println(line)
		}
		return
	}

	for i := 0; i < maxLines; i++ {
		var logoPart string
		if i < len(coloredLogoLines) {
			logoPart = coloredLogoLines[i]
		} else {
			logoPart = strings.Repeat(" ", logoWidth)
		}

		var rawLogoLen int
		if i < len(logoLines) {
			rawLogoLen = utf8.RuneCountInString(logoLines[i])
		} else {
			rawLogoLen = logoWidth
		}

		spacing := strings.Repeat(" ", logoWidth-rawLogoLen+3)

		var infoPart string
		if i < len(infoLines) {
			infoPart = infoLines[i]
		} else {
			infoPart = ""
		}

		// Ensure it doesn't overflow the screen width
		if logoWidth+3+visualLen(infoPart) > termWidth {
			allowedWidth := termWidth - logoWidth - 4
			if allowedWidth > 10 {
				infoPart = truncateAnsi(infoPart, allowedWidth)
			}
		}

		fmt.Printf("%s%s%s\n", logoPart, spacing, infoPart)
	}
	fmt.Println() // Extra line break for bit space at the end of CLI
}

// --- Display Functions ---

// DisplaySystemInfo formats and prints the system info (exported).
func DisplaySystemInfo(info *gather.SystemInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	var infoLines []string

	username := getUsername()
	hostname := info.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Accent + strings.Repeat("-", len(username)+1+len(hostname)) + theme.Reset
	infoLines = append(infoLines, separator)

	items := []struct{ Key, Value string }{
		{"OS", info.OS},
		{"Host", info.Host},
		{"Kernel", info.Kernel},
		{"Uptime", info.Uptime},
		{"Packages", info.Packages},
		{"Shell", info.Shell},
		{"Resolution", info.Resolution},
		{"DE", info.DE},
		{"WM", info.WindowManager},
		{"Terminal", info.Terminal},
		{"CPU", info.CPU},
		{"GPU", info.GPU},
		{"Memory", info.RAM},
		{"Swap", info.Swap},
		{"Disk", info.Disk},
		{"Local IP", info.IPAddress},
		{"Locale", info.Locale},
		{"Languages", info.Languages},
		{"Open Ports", info.OpenPorts},
		{"Temperature", info.Temperature},
		{"Go Version", info.Go},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "Unknown" && val != "None" && val != "N/A" && val != "None detected" && val != "0GB/0GB (0.0%)" && val != "0GB / 0GB (0.0%)" {
			// Pre-truncate the value so it fits perfectly on the right panel
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}
			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// GetThemeForGPUVendor returns the authentic brand color scheme for a GPU vendor (strictly vendor-based, never distro-based).
func GetThemeForGPUVendor(vendorKey string) Theme {
	switch vendorKey {
	case "nvidia":
		return Theme{
			Category: "\033[38;5;82m",  // Nvidia Bright Green
			Key:      "\033[38;5;82m",
			Value:    "\033[38;5;249m", // Light Gray
			Accent:   "\033[38;5;82m",  // Nvidia Bright Green
			Reset:    "\033[0m",
		}
	case "amd":
		return Theme{
			Category: "\033[38;5;196m", // Radeon Bright Red
			Key:      "\033[38;5;196m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;196m", // Radeon Bright Red
			Reset:    "\033[0m",
		}
	case "intel":
		return Theme{
			Category: "\033[38;5;33m",  // Intel Blue
			Key:      "\033[38;5;39m",  // Electric Sky Blue
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;33m",  // Intel Blue
			Reset:    "\033[0m",
		}
	case "qualcomm":
		return Theme{
			Category: "\033[38;5;197m", // Qualcomm Crimson/Red
			Key:      "\033[38;5;208m", // Snapdragon Orange
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;197m",
			Reset:    "\033[0m",
		}
	case "broadcom":
		return Theme{
			Category: "\033[38;5;196m", // Broadcom Red
			Key:      "\033[38;5;196m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;196m",
			Reset:    "\033[0m",
		}
	case "arm":
		return Theme{
			Category: "\033[38;5;51m",  // ARM Electric Cyan
			Key:      "\033[38;5;51m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;51m",
			Reset:    "\033[0m",
		}
	case "samsung":
		return Theme{
			Category: "\033[38;5;33m",  // Samsung Blue
			Key:      "\033[38;5;39m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;33m",
			Reset:    "\033[0m",
		}
	case "apple":
		return Theme{
			Category: "\033[38;5;255m", // Apple Bright Silver
			Key:      "\033[38;5;255m",
			Value:    "\033[38;5;250m",
			Accent:   "\033[38;5;255m",
			Reset:    "\033[0m",
		}
	case "mediatek":
		return Theme{
			Category: "\033[38;5;214m", // MediaTek Orange
			Key:      "\033[38;5;214m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;214m",
			Reset:    "\033[0m",
		}
	case "rockchip":
		return Theme{
			Category: "\033[38;5;39m",  // Rockchip Cyan
			Key:      "\033[38;5;39m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;39m",
			Reset:    "\033[0m",
		}
	case "riscv":
		return Theme{
			Category: "\033[38;5;45m",  // RISC-V Teal
			Key:      "\033[38;5;45m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;45m",
			Reset:    "\033[0m",
		}
	default: // generic GPU hardware (strictly vendor-neutral hardware cyan, not distro)
		return Theme{
			Category: "\033[38;5;45m",  // Hardware Neon Cyan
			Key:      "\033[38;5;45m",
			Value:    "\033[38;5;249m",
			Accent:   "\033[38;5;45m",
			Reset:    "\033[0m",
		}
	}
}

// DisplayGPUInfo formats and prints detailed GPU and graphics API information (exported).
func DisplayGPUInfo(info *gather.GPUDetails, theme Theme) {
	vendorKey := strings.ToLower(info.Vendor)
	if strings.Contains(vendorKey, "nvidia") || strings.Contains(vendorKey, "geforce") {
		vendorKey = "nvidia"
	} else if strings.Contains(vendorKey, "intel") {
		vendorKey = "intel"
	} else if strings.Contains(vendorKey, "amd") || strings.Contains(vendorKey, "advanced micro") || strings.Contains(vendorKey, "radeon") || strings.Contains(vendorKey, "ati") {
		vendorKey = "amd"
	} else if strings.Contains(vendorKey, "qualcomm") || strings.Contains(vendorKey, "adreno") || strings.Contains(vendorKey, "snapdragon") {
		vendorKey = "qualcomm"
	} else if strings.Contains(vendorKey, "apple") {
		vendorKey = "apple"
	} else if strings.Contains(vendorKey, "broadcom") || strings.Contains(vendorKey, "videocore") {
		vendorKey = "broadcom"
	} else if strings.Contains(vendorKey, "arm") || strings.Contains(vendorKey, "mali") {
		vendorKey = "arm"
	} else if strings.Contains(vendorKey, "samsung") || strings.Contains(vendorKey, "exynos") {
		vendorKey = "samsung"
	} else if strings.Contains(vendorKey, "mediatek") || strings.Contains(vendorKey, "dimensity") {
		vendorKey = "mediatek"
	} else if strings.Contains(vendorKey, "rockchip") {
		vendorKey = "rockchip"
	} else if strings.Contains(vendorKey, "riscv") || strings.Contains(vendorKey, "starfive") || strings.Contains(vendorKey, "sifive") {
		vendorKey = "riscv"
	}

	if vendorKey == "" || vendorKey == "unknown" {
		lowerName := strings.ToLower(info.Name)
		if strings.Contains(lowerName, "nvidia") || strings.Contains(lowerName, "geforce") {
			vendorKey = "nvidia"
		} else if strings.Contains(lowerName, "intel") {
			vendorKey = "intel"
		} else if strings.Contains(lowerName, "amd") || strings.Contains(lowerName, "radeon") || strings.Contains(lowerName, "ati") {
			vendorKey = "amd"
		} else if strings.Contains(lowerName, "apple") {
			vendorKey = "apple"
		} else if strings.Contains(lowerName, "broadcom") || strings.Contains(lowerName, "videocore") {
			vendorKey = "broadcom"
		} else if strings.Contains(lowerName, "qualcomm") || strings.Contains(lowerName, "adreno") || strings.Contains(lowerName, "snapdragon") {
			vendorKey = "qualcomm"
		} else if strings.Contains(lowerName, "arm") || strings.Contains(lowerName, "mali") {
			vendorKey = "arm"
		} else if strings.Contains(lowerName, "samsung") || strings.Contains(lowerName, "exynos") {
			vendorKey = "samsung"
		} else if strings.Contains(lowerName, "mediatek") || strings.Contains(lowerName, "dimensity") {
			vendorKey = "mediatek"
		} else if strings.Contains(lowerName, "rockchip") {
			vendorKey = "rockchip"
		} else if strings.Contains(lowerName, "riscv") || strings.Contains(lowerName, "starfive") || strings.Contains(lowerName, "sifive") {
			vendorKey = "riscv"
		}
	}

	if theme != BlankTheme {
		theme = GetThemeForGPUVendor(vendorKey)
	}

	termWidth := getTerminalWidth()
	logoWidth := 0

	var logoLines []string
	var exists bool
	logoLines, exists = GPULogos[vendorKey]
	if !exists {
		logoLines = GPULogos["generic"]
	}

	for _, l := range logoLines {
		runesCount := utf8.RuneCountInString(l)
		if runesCount > logoWidth {
			logoWidth = runesCount
		}
	}

	var infoLines []string

	username := getUsername()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Accent + strings.Repeat("-", len(username)+1+len(hostname)) + theme.Reset
	infoLines = append(infoLines, separator)

	var vramCombined string
	if info.VRAMUsed != "" && info.VRAMTotal != "" {
		vramCombined = fmt.Sprintf("%s / %s", info.VRAMUsed, info.VRAMTotal)
		if info.VRAMType != "" {
			vramCombined += fmt.Sprintf(" (%s)", info.VRAMType)
		}
	} else if info.VRAMTotal != "" {
		vramCombined = info.VRAMTotal
		if info.VRAMType != "" {
			vramCombined += fmt.Sprintf(" (%s)", info.VRAMType)
		}
	} else if info.VRAMUsed != "" {
		vramCombined = info.VRAMUsed
	}

	driverCombined := info.Driver
	if info.DriverVersion != "" {
		if driverCombined != "" && !strings.Contains(driverCombined, info.DriverVersion) {
			driverCombined = fmt.Sprintf("%s (%s)", driverCombined, info.DriverVersion)
		} else if driverCombined == "" {
			driverCombined = info.DriverVersion
		}
	}

	vulkanCombined := info.Vulkan
	if info.VulkanDriver != "" && info.Vulkan != "" && !strings.Contains(info.Vulkan, info.VulkanDriver) {
		vulkanCombined = fmt.Sprintf("%s (%s)", info.Vulkan, info.VulkanDriver)
	}

	items := []struct{ Key, Value string }{
		{"GPU Model", info.Name},
		{"Vendor", info.Vendor},
		{"Device ID", info.DeviceID},
		{"Subsystem", info.Subsystem},
		{"PCI Bus", info.PCIBus},
		{"Driver", driverCombined},
		{"Power State", info.PowerState},
		{"GPU Clock", info.CurrentClock},
		{"Video Memory", vramCombined},
		{"VRAM Free", info.VRAMFree},
		{"Displays", info.ActiveDisplays},
		{"Temperature", info.Temperature},
		{"OpenGL", info.OpenGL},
		{"GL Renderer", info.OpenGLRenderer},
		{"GL Core", info.OpenGLCore},
		{"GL ES", info.OpenGLES},
		{"Vulkan", vulkanCombined},
		{"OpenCL", info.OpenCL},
		{"CUDA", info.CUDA},
		{"Metal", info.Metal},
		{"DirectX", info.DirectX},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "Unknown" && val != "None" && val != "N/A" {
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}
			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	// Draw side-by-side or stacked layout using the vendor logo
	sideBySide := termWidth >= logoWidth+30
	if sideBySide {
		maxLines := len(logoLines)
		if len(infoLines) > maxLines {
			maxLines = len(infoLines)
		}

		for i := 0; i < maxLines; i++ {
			logoLine := ""
			runeLen := 0
			if i < len(logoLines) {
				logoLine = theme.Accent + logoLines[i] + theme.Reset
				runeLen = utf8.RuneCountInString(logoLines[i])
			} else {
				logoLine = strings.Repeat(" ", logoWidth)
			}

			if runeLen < logoWidth && i < len(logoLines) {
				logoLine += strings.Repeat(" ", logoWidth-runeLen)
			}

			infoLine := ""
			if i < len(infoLines) {
				infoLine = infoLines[i]
			}

			fmt.Printf("%s   %s\n", logoLine, infoLine)
		}
	} else {
		for _, l := range logoLines {
			fmt.Println(theme.Accent + l + theme.Reset)
		}
		fmt.Println()
		for _, l := range infoLines {
			fmt.Println(l)
		}
	}
}

// DisplayKernelInfo formats and prints essential kernel parameters, modules, security, and mitigations (exported).
func DisplayKernelInfo(info *gather.KernelDetails, theme Theme) {
	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	var infoLines []string

	username := getUsername()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s (Kernel)", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Accent + strings.Repeat("-", len(username)+1+len(hostname)+9) + theme.Reset
	infoLines = append(infoLines, separator)

	// Build summary
	var buildParts []string
	if info.Compiler != "" {
		buildParts = append(buildParts, info.Compiler)
	}
	if info.Preemption != "" {
		buildParts = append(buildParts, info.Preemption)
	}
	if info.BuildDate != "" {
		buildParts = append(buildParts, info.BuildDate)
	}
	buildCombined := strings.Join(buildParts, ", ")

	// Modules combined (loaded + memory + unloaded)
	var modulesCombined string
	if info.LoadedModulesCount > 0 {
		if info.LoadedModulesMemory != "" {
			modulesCombined = fmt.Sprintf("%d loaded (%s)", info.LoadedModulesCount, info.LoadedModulesMemory)
		} else {
			modulesCombined = fmt.Sprintf("%d loaded", info.LoadedModulesCount)
		}
		if info.UnloadedModulesCount > 0 {
			modulesCombined += fmt.Sprintf(" / %d unloaded", info.UnloadedModulesCount)
		}
	}

	kernelCombined := info.Release
	if info.Architecture != "" {
		kernelCombined = fmt.Sprintf("%s (%s)", info.Release, info.Architecture)
	}

	cmdLine := info.CleanCommandLine
	if cmdLine == "" {
		cmdLine = info.CommandLine
	}

	items := []struct{ Key, Value string }{
		{"Kernel", kernelCombined},
		{"Build", buildCombined},
		{"Command Line", cmdLine},
		{"Modules", modulesCombined},
		{"Dependencies", info.TopDependencies},
		{"Kernel Taint", info.TaintStatus},
		{"Secure Boot", info.SecureBoot},
		{"Lockdown", info.Lockdown},
		{"Security", info.SecurityLSM},
		{"KASLR", info.KASLR},
		{"eBPF JIT", info.EBPFJIT},
		{"CPU Scaling", info.CPUScaling},
		{"Clock Source", info.ClockSource},
		{"Mitigations", info.Mitigations},
		{"Kernel Updates", info.UpdateStatus},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "Unknown" && val != "None" && val != "N/A" {
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}
			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// DisplayProcessList formats and prints the process list (Wider & Centered Title).
func DisplayProcessList(processList []gather.ProcessInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	pidWidth := 8
	cpuWidth := 8
	ramWidth := 8

	sideBySide := termWidth >= logoWidth+45

	var rightPanelWidth int
	if sideBySide {
		rightPanelWidth = termWidth - logoWidth - 3
	} else {
		rightPanelWidth = termWidth
	}

	nameWidth := rightPanelWidth - pidWidth - cpuWidth - ramWidth - 5
	if nameWidth < 15 {
		nameWidth = 15
	}

	totalWidth := pidWidth + nameWidth + cpuWidth + ramWidth + 3
	if totalWidth > rightPanelWidth {
		nameWidth -= (totalWidth - rightPanelWidth)
	}
	if nameWidth < 15 {
		nameWidth = 15
	}
	totalWidth = pidWidth + nameWidth + cpuWidth + ramWidth + 3

	var infoLines []string

	username := getUsername()
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s (Processes)", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)+12) + theme.Reset
	infoLines = append(infoLines, separator)

	tableHeader := fmt.Sprintf("%s%-*s %-*s %*s %*s%s",
		theme.Key, pidWidth, "PID", nameWidth, "NAME", cpuWidth, "CPU%", ramWidth, "RAM", theme.Reset)
	infoLines = append(infoLines, tableHeader)

	tableSep := theme.Value + strings.Repeat("─", totalWidth) + theme.Reset
	infoLines = append(infoLines, tableSep)

	limit := 18
	if len(processList) < limit {
		limit = len(processList)
	}

	for _, p := range processList[:limit] {
		var ramStr string
		ramMB := float64(p.RAM) / (1024 * 1024)
		if ramMB < 1.0 {
			ramKB := float64(p.RAM) / 1024
			ramStr = fmt.Sprintf("%.0fK", ramKB)
		} else if ramMB < 1024 {
			ramStr = fmt.Sprintf("%.0fM", ramMB)
		} else {
			ramGB := ramMB / 1024
			ramStr = fmt.Sprintf("%.1fG", ramGB)
		}

		cpuColor := theme.Value
		if p.CPU > 20.0 {
			cpuColor = "\033[1;31m" // Bold Red
		} else if p.CPU > 5.0 {
			cpuColor = "\033[1;33m" // Bold Yellow
		}

		ramColor := theme.Value
		if p.RAM > 500*1024*1024 {
			ramColor = "\033[1;31m"
		} else if p.RAM > 150*1024*1024 {
			ramColor = "\033[1;33m"
		}

		row := fmt.Sprintf("%s%-*d %s%-*s %s%*.1f%% %s%*s%s",
			theme.Value, pidWidth, p.PID,
			theme.Key, nameWidth, truncateString(p.Name, nameWidth),
			cpuColor, cpuWidth-1, p.CPU,
			ramColor, ramWidth, ramStr,
			theme.Reset)
		infoLines = append(infoLines, row)
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// DisplayNetworkInfo formats and prints detailed network info (Exported)
func DisplayNetworkInfo(info *gather.NetworkInfo, theme Theme) {

	distroKey := getDistroKey()
	theme = GetThemeForDistro(distroKey, theme == BlankTheme)

	termWidth := getTerminalWidth()
	logoWidth := 0
	if logoLines, exists := DistroLogos[distroKey]; exists {
		for _, l := range logoLines {
			runesCount := utf8.RuneCountInString(l)
			if runesCount > logoWidth {
				logoWidth = runesCount
			}
		}
	} else {
		logoWidth = 16
	}

	var infoLines []string

	username := getUsername()
	hostname := info.Hostname
	if hostname == "" {
		hostname = "unknown"
	}

	header := fmt.Sprintf("%s%s%s@%s%s%s (Network)", theme.Key, username, theme.Reset, theme.Key, hostname, theme.Reset)
	infoLines = append(infoLines, header)

	separator := theme.Value + strings.Repeat("-", len(username)+1+len(hostname)+10) + theme.Reset
	infoLines = append(infoLines, separator)

	var primaryIfaceStr string
	if info.PrimaryIface != "" {
		if info.IfaceType != "" {
			primaryIfaceStr = fmt.Sprintf("%s (%s)", info.PrimaryIface, info.IfaceType)
		} else {
			primaryIfaceStr = info.PrimaryIface
		}
	}

	var wifiSignalStr string
	if info.Wifi != nil && (info.Wifi.SignalPerc > 0 || info.Wifi.SignalDBm != 0) {
		if info.Wifi.SignalPerc > 0 && info.Wifi.SignalDBm != 0 {
			wifiSignalStr = fmt.Sprintf("%d%% (%d dBm)", info.Wifi.SignalPerc, info.Wifi.SignalDBm)
		} else if info.Wifi.SignalPerc > 0 {
			wifiSignalStr = fmt.Sprintf("%d%%", info.Wifi.SignalPerc)
		} else {
			wifiSignalStr = fmt.Sprintf("%d dBm", info.Wifi.SignalDBm)
		}
	}

	var wifiFreqChannel string
	if info.Wifi != nil {
		if info.Wifi.Freq != "" && info.Wifi.Channel != "" {
			wifiFreqChannel = fmt.Sprintf("%s (Ch %s)", info.Wifi.Freq, info.Wifi.Channel)
		} else if info.Wifi.Freq != "" {
			wifiFreqChannel = info.Wifi.Freq
		} else if info.Wifi.Channel != "" {
			wifiFreqChannel = fmt.Sprintf("Channel %s", info.Wifi.Channel)
		}
	}

	var trafficStr string
	if info.RxTotal != "" && info.TxTotal != "" && info.RxTotal != "0 B" {
		trafficStr = fmt.Sprintf("▼ %s │ ▲ %s", info.RxTotal, info.TxTotal)
	} else if info.IOCounters != "" && info.IOCounters != "N/A" {
		trafficStr = info.IOCounters
	}

	var dnsStr string
	if len(info.DNSServers) > 0 {
		dnsStr = strings.Join(info.DNSServers, ", ")
	}

	var locationStr string
	if info.City != "" && info.Country != "" && info.City != "Unknown" && info.Country != "Unknown" && info.City != "Error" && info.Country != "Error" {
		locationStr = fmt.Sprintf("%s, %s", info.City, info.Country)
	} else if info.Country != "" && info.Country != "Unknown" && info.Country != "Error" {
		locationStr = info.Country
	} else if info.City != "" && info.City != "Unknown" && info.City != "Error" {
		locationStr = info.City
	}

	var otherIfaces []string
	for _, iface := range info.Interfaces {
		if iface.Name != info.PrimaryIface && iface.State == "UP" && iface.Type != "Loopback" {
			addr := iface.IPv4
			if addr == "" {
				addr = iface.Type
			}
			otherIfaces = append(otherIfaces, fmt.Sprintf("%s (%s)", iface.Name, addr))
		}
	}
	var otherIfacesStr string
	if len(otherIfaces) > 0 {
		otherIfacesStr = strings.Join(otherIfaces, ", ")
	}

	var wifiSSID, wifiBSSID, wifiBitrate, wifiSecurity string
	if info.Wifi != nil {
		wifiSSID = info.Wifi.SSID
		wifiBSSID = info.Wifi.BSSID
		wifiBitrate = info.Wifi.Bitrate
		wifiSecurity = info.Wifi.Security
	}

	items := []struct{ Key, Value string }{
		{"Hostname", info.Hostname},
		{"Primary Interface", primaryIfaceStr},
		{"IPv4 Address", info.PrivateIP},
		{"IPv6 Address", info.IPv6Address},
		{"MAC Address", info.MACAddress},
		{"Default Gateway", info.Gateway},
		{"DNS Servers", dnsStr},
		{"Wi-Fi SSID", wifiSSID},
		{"Wi-Fi BSSID", wifiBSSID},
		{"Wi-Fi Signal", wifiSignalStr},
		{"Wi-Fi Channel/Band", wifiFreqChannel},
		{"Wi-Fi Link Speed", wifiBitrate},
		{"Wi-Fi Security", wifiSecurity},
		{"Public IP", info.PublicIP},
		{"ISP / Org", info.ISP},
		{"Location", locationStr},
		{"Ping (1.1.1.1)", info.Ping},
		{"Network Traffic", trafficStr},
		{"Packets & Errors", info.PacketStats},
		{"Active Sockets", info.SocketStats},
		{"Other Interfaces", otherIfacesStr},
		{"Proxy", info.Proxy},
	}

	for _, item := range items {
		val := strings.TrimSpace(item.Value)
		if val != "" && val != "N/A" && val != "Error" && val != "," && val != "Error, Error" && val != "Unknown" && val != "None" {
			valAllowed := termWidth - logoWidth - 3 - len(item.Key) - 4
			if valAllowed > 5 {
				val = truncateString(val, valAllowed)
			}

			line := fmt.Sprintf("%s%s%s: %s%s%s", theme.Key, item.Key, theme.Reset, theme.Value, val, theme.Reset)
			infoLines = append(infoLines, line)
		}
	}

	blocks := getColorBlocks(theme)
	if blocks != "" {
		infoLines = append(infoLines, "")
		blockLines := strings.Split(blocks, "\n")
		for _, bl := range blockLines {
			infoLines = append(infoLines, bl)
		}
	}

	DisplayLayout(distroKey, infoLines, theme)
}

// PrintHelp prints a distro-themed, highly aesthetic help screen to stdout.
func PrintHelp(version string, noColor bool) {
	distroKey := getDistroKey()
	theme := GetThemeForDistro(distroKey, noColor)

	title := " KernelView Go - System Fetch & Live Dashboard "
	verStr := fmt.Sprintf(" Version: %s (Detected: %s)", version, distroKey)

	// Print visual box header themed with distro primary colors
	fmt.Printf("%s╭────────────────────────────────────────────────────────╮%s\n", theme.Accent, theme.Reset)
	fmt.Printf("%s│%s%s%s%s%s│%s\n",
		theme.Accent,
		theme.Reset,
		"\033[1m",
		fmt.Sprintf("  %-54s", title),
		theme.Reset,
		theme.Accent,
		theme.Reset,
	)
	fmt.Printf("%s│%s%s%s%s│%s\n",
		theme.Accent,
		theme.Reset,
		theme.Value,
		fmt.Sprintf("  %-54s", verStr),
		theme.Accent,
		theme.Reset,
	)
	fmt.Printf("%s╰────────────────────────────────────────────────────────╯%s\n\n", theme.Accent, theme.Reset)

	// Section 1: Usage
	fmt.Printf("%sUSAGE:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  kernelview [flags]\n\n")

	// Section 2: Display Modes
	fmt.Printf("%sDISPLAY MODES:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-l, --live%s         %sStart real-time TUI dashboard (interactive, multi-tab)%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-p, --process%s      %sDisplay static list of running processes%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-n, --network%s      %sDisplay detailed static network interfaces & stats%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-g, --gpu%s          %sDisplay detailed static GPU & graphics API report%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-k, --kernel%s       %sDisplay detailed kernel parameters, modules, security & mitigations%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s[default]%s          %sDisplay system fetch (fastfetch-like distro ASCII logo & specs)%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 3: Configuration Options
	fmt.Printf("%sCONFIGURATION OPTIONS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-f, --fast%s         %sRun system fetch in fast mode (skips slow subsystem checks)%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s--json%s             %sOutput information as structured JSON%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s--no-color%s         %sDisable all ANSI color formatting%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 4: Information Flags
	fmt.Printf("%sINFO FLAGS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s-v, --version%s      %sPrint version and build information%s\n", theme.Key, theme.Reset, theme.Value, theme.Reset)
	fmt.Printf("  %s-h, --help%s         %sPrint this aesthetic help menu%s\n\n", theme.Key, theme.Reset, theme.Value, theme.Reset)

	// Section 5: TUI Interactive Keys
	fmt.Printf("%sTUI INTERACTIVE KEYS:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  %s[1]%s Dashboard Tab   %s[2]%s Processes Tab   %s[3]%s Network Tab   %s[4]%s CPU Cores Tab   %s[5]%s Disk/IO Tab   %s[Q]%s Quit\n\n",
		theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset, theme.Key, theme.Reset)

	// Section 6: Examples
	fmt.Printf("%sEXAMPLES:%s\n", theme.Accent, theme.Reset)
	fmt.Printf("  $ kernelview --live            %s# Open the live TUI dashboard%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -p --json         %s# Fetch running processes in JSON format%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -g                %s# Display detailed GPU and graphics API report%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -k                %s# Display detailed kernel security, modules & mitigations%s\n", "\033[90m", theme.Reset)
	fmt.Printf("  $ kernelview -f                %s# Fast system fetch with ASCII logo%s\n\n", "\033[90m", theme.Reset)

	// Footer with Color Grid blocks
	blocks := getColorBlocks(theme)
	if blocks != "" {
		fmt.Printf("%s\n\n", blocks)
	}
}
