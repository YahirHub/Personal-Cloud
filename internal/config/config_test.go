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

func TestLoadHTTPSForcesSecureCookies(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_REQUIRE_HTTPS", "true")
	t.Setenv("APP_COOKIE_SECURE", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.CookieSecure {
		t.Fatal("HTTPS obligatorio debe forzar cookies Secure")
	}
}

func TestLoadTLSRequiresCertificatePair(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_TLS_CERT_FILE", "cert.pem")
	t.Setenv("APP_TLS_KEY_FILE", "")
	if _, err := Load(); err == nil {
		t.Fatal("esperaba error al configurar solo el certificado")
	}
}

func TestLoadAppURLNormalizesAndValidates(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	t.Setenv("APP_URL", "https://ncloud.admvo.org/")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppURL != "https://ncloud.admvo.org" {
		t.Fatalf("APP_URL inesperada: %q", cfg.AppURL)
	}
}

func TestLoadRejectsInvalidAppURL(t *testing.T) {
	t.Setenv("APP_DATA_DIR", t.TempDir())
	for _, value := range []string{"ncloud.admvo.org", "ftp://ncloud.admvo.org", "https://user:pass@ncloud.admvo.org", "https://ncloud.admvo.org/?x=1"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_URL", value)
			if _, err := Load(); err == nil {
				t.Fatalf("APP_URL %q debía rechazarse", value)
			}
		})
	}
}
