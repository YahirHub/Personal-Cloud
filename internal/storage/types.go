package storage

import "time"

type DiscoveredVolume struct {
	PersistentID   string
	HardwareID     string
	IdentityStable bool
	Name           string
	Label          string
	Platform       string
	Device         string
	VolumeName     string
	MountPoint     string
	FSType         string
	Mounted        bool
	ReadOnly       bool
	System         bool
	Removable      bool
	Capacity       uint64
	Free           uint64
}

// Presence resume identidades detectadas sin calcular capacidad ni recorrer contenido.
// Hardware cuenta cuántos volúmenes comparten el mismo identificador para evitar
// reconexiones ambiguas cuando un mismo disco físico contiene varias particiones.
type Presence struct {
	Persistent map[string]struct{}
	Hardware   map[string]int
}

type View struct {
	ID                 string
	PersistentID       string
	HardwareID         string
	IdentityStable     bool
	Name               string
	Label              string
	Platform           string
	Device             string
	VolumeName         string
	MountPoint         string
	FSType             string
	Category           string
	VirtualRoot        string
	IdleTimeoutSeconds int
	AutoUnmount        bool
	ReadOnly           bool
	Registered         bool
	Online             bool
	Mounted            bool
	System             bool
	Removable          bool
	Capacity           uint64
	Free               uint64
	ActiveHandles      int
	LastActivity       time.Time
	Status             string
	Error              string
}

type RegisterInput struct {
	PersistentID       string
	Name               string
	Category           string
	VirtualRoot        string
	IdleTimeoutSeconds int
	AutoUnmount        bool
	ReadOnly           bool
}

func CategoryLabel(value string) string {
	switch value {
	case "documents":
		return "Documentos"
	case "photos":
		return "Fotos"
	case "multimedia":
		return "Multimedia"
	default:
		return "Mixto"
	}
}
