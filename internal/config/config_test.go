package config

import "testing"

func TestParseCIDRs(t *testing.T) {
	nets, err := parseCIDRs("127.0.0.1/32, 10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	if len(nets) != 2 {
		t.Fatalf("esperaba 2 redes, obtuvo %d", len(nets))
	}
}
