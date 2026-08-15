package app

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDownloadTicketIsOpaqueAndBoundToCiphertext(t *testing.T) {
	a := &App{}
	for i := range a.downloadSecret {
		a.downloadSecret[i] = byte(i + 1)
	}
	input := downloadTicket{FileID: "archivo-secreto-123", UserID: "usuario-456", Expires: time.Now().Add(time.Minute).Unix()}
	token, err := a.encryptDownloadTicket(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(token, input.FileID) || strings.Contains(token, input.UserID) {
		t.Fatal("el ticket no debe revelar identificadores en texto claro")
	}
	got, err := a.decryptDownloadTicket(token)
	if err != nil {
		t.Fatal(err)
	}
	if got != input {
		t.Fatalf("ticket=%+v want=%+v", got, input)
	}

	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatal(err)
	}
	payload[len(payload)-1] ^= 0x01
	tampered := base64.RawURLEncoding.EncodeToString(payload)
	if _, err := a.decryptDownloadTicket(tampered); err == nil {
		t.Fatal("un ticket alterado debe fallar autenticación")
	}
}

func TestRemoteDownloadRequiresHTTPS(t *testing.T) {
	a := &App{}
	req := httptest.NewRequest(http.MethodPost, "http://example.test/api/descargas", nil)
	req.RemoteAddr = "203.0.113.10:4321"
	rr := httptest.NewRecorder()
	a.downloadTicketPost(rr, req)
	if rr.Code != http.StatusUpgradeRequired {
		t.Fatalf("status=%d want=%d", rr.Code, http.StatusUpgradeRequired)
	}
}
