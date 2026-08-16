package app

import (
	"testing"

	storagepkg "personalcloud/internal/storage"
)

func TestStorageUsageFromViewsAggregatesOnlyRegisteredOnlineUnits(t *testing.T) {
	views := []storagepkg.View{
		{ID: "one", Name: "SSD", Registered: true, Online: true, Capacity: 1000, Free: 250, VirtualRoot: "ssd"},
		{ID: "two", Name: "HDD", Registered: true, Online: true, Capacity: 3000, Free: 1000, VirtualRoot: "hdd"},
		{ID: "offline", Name: "Fuera", Registered: true, Online: false, Capacity: 9000, Free: 9000},
		{ID: "candidate", Name: "Sin registrar", Registered: false, Online: true, Capacity: 8000, Free: 4000},
		{ID: "unknown", Name: "Sin capacidad", Registered: true, Online: true, Capacity: 0, Free: 0},
	}

	summary, items := storageUsageFromViews(views)
	if summary.Total != 4000 || summary.Free != 1250 || summary.Used != 2750 {
		t.Fatalf("resumen inesperado: %+v", summary)
	}
	if summary.OnlineUnits != 2 {
		t.Fatalf("se esperaban 2 unidades contabilizadas, se obtuvieron %d", summary.OnlineUnits)
	}
	if summary.PercentUsed != 69 {
		t.Fatalf("porcentaje agregado inesperado: %d", summary.PercentUsed)
	}
	if len(items) != 2 {
		t.Fatalf("se esperaban 2 detalles de unidad, se obtuvieron %d", len(items))
	}
	if items[0].Name != "HDD" || items[1].Name != "SSD" {
		t.Fatalf("las unidades deben ordenarse por nombre: %+v", items)
	}
	if items[0].Used != 2000 || items[0].Free != 1000 || items[0].Capacity != 3000 {
		t.Fatalf("detalle HDD inesperado: %+v", items[0])
	}
}

func TestStorageUsageFromViewsBoundsInvalidFreeSpace(t *testing.T) {
	summary, items := storageUsageFromViews([]storagepkg.View{{
		ID: "unit", Name: "Unidad", Registered: true, Online: true, Capacity: 1024, Free: 4096,
	}})
	if summary.Total != 1024 || summary.Free != 1024 || summary.Used != 0 || summary.PercentUsed != 0 {
		t.Fatalf("el espacio libre debe limitarse a la capacidad: %+v", summary)
	}
	if len(items) != 1 || items[0].Free != 1024 || items[0].Used != 0 {
		t.Fatalf("detalle de unidad inválido: %+v", items)
	}
}
