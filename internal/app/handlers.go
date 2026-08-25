package app

import (
	"errors"
	"net/http"
	"strings"

	"personalcloud/internal/auth"
	"personalcloud/internal/store"
)

const maxFormBytes = 64 << 10

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	exists, err := a.store.AdminExists(r.Context())
	if err != nil {
		http.Error(w, "No se pudo comprobar el estado del servidor.", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	user, _ := a.currentUser(r)
	if user == nil {
		http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
		return
	}
	if !user.OnboardingCompleted {
		http.Redirect(w, r, "/bienvenida", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/inicio", http.StatusSeeOther)
}

func (a *App) setupGet(w http.ResponseWriter, r *http.Request) {
	exists, err := a.store.AdminExists(r.Context())
	if err != nil {
		http.Error(w, "No se pudo comprobar la configuración.", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
		return
	}
	data := a.csrfData(w, r, pageData{
		Title:       "Configuración inicial",
		Description: "Crea la primera cuenta administradora usando el código mostrado en el log del servidor.",
		CurrentPath: "/setup",
	})
	a.render(w, http.StatusOK, "setup", data)
}

func (a *App) setupPost(w http.ResponseWriter, r *http.Request) {
	exists, err := a.store.AdminExists(r.Context())
	if err != nil {
		http.Error(w, "No se pudo comprobar la configuración.", http.StatusInternalServerError)
		return
	}
	if exists {
		http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
		return
	}

	if !a.parseProtectedForm(w, r) {
		return
	}
	data := pageData{Title: "Configuración inicial", Description: "Crea la primera cuenta administradora.", CurrentPath: "/setup"}
	ip := a.clientIP(r)
	key := "setup:ip:" + ip
	if ok, wait := a.limiter.Allow(key, setupPolicy); !ok {
		a.tooMany(w, r, "setup", data, wait)
		return
	}

	if !secureEqual(strings.ToUpper(strings.TrimSpace(r.FormValue("setup_code"))), a.setupCode) {
		_ = a.store.Audit(r.Context(), "", "setup", "codigo_invalido", ip)
		data.Error = "El código de configuración no es válido. Revisa el código actual en el log del servidor."
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusUnauthorized, "setup", data)
		return
	}

	username, err := auth.NormalizeUsername(r.FormValue("username"))
	if err != nil {
		data.Error = err.Error()
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusBadRequest, "setup", data)
		return
	}
	password := r.FormValue("password")
	confirmation := r.FormValue("password_confirmation")
	if password != confirmation {
		data.Error = "Las contraseñas no coinciden."
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusBadRequest, "setup", data)
		return
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		data.Error = err.Error()
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusBadRequest, "setup", data)
		return
	}

	user, err := a.store.CreateFirstAdmin(r.Context(), username, passwordHash)
	if err != nil {
		a.logger.Warn("no se pudo crear administrador", "error", err)
		data.Error = "No se pudo crear la cuenta administradora. Puede que el servidor ya haya sido configurado."
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusConflict, "setup", data)
		return
	}
	if err := a.createLoginSession(w, r, user.ID); err != nil {
		a.logger.Error("administrador creado pero no se pudo iniciar sesión", "error", err)
		http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
		return
	}
	a.setupCode = ""
	a.limiter.Reset(key)
	_ = a.store.Audit(r.Context(), user.ID, "setup", "administrador_creado", ip)
	http.Redirect(w, r, "/bienvenida", http.StatusSeeOther)
}

func (a *App) loginGet(w http.ResponseWriter, r *http.Request) {
	setAuthNoStore(w)
	exists, err := a.store.AdminExists(r.Context())
	if err != nil {
		http.Error(w, "No se pudo comprobar la configuración.", http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	if user, _ := a.currentUser(r); user != nil {
		if user.OnboardingCompleted {
			http.Redirect(w, r, "/inicio", http.StatusSeeOther)
		} else {
			http.Redirect(w, r, "/bienvenida", http.StatusSeeOther)
		}
		return
	}
	data := a.csrfData(w, r, pageData{Title: "Iniciar sesión", Description: "Accede a tu nube personal.", CurrentPath: "/iniciar-sesion"})
	a.render(w, http.StatusOK, "login", data)
}

func (a *App) loginPost(w http.ResponseWriter, r *http.Request) {
	setAuthNoStore(w)
	if !a.parseProtectedForm(w, r) {
		return
	}
	data := pageData{Title: "Iniciar sesión", Description: "Accede a tu nube personal.", CurrentPath: "/iniciar-sesion"}
	ip := a.clientIP(r)
	ipKey := "login:ip:" + ip
	if ok, wait := a.limiter.Allow(ipKey, loginIPPolicy); !ok {
		a.tooMany(w, r, "login", data, wait)
		return
	}

	username, normalizeErr := auth.NormalizeUsername(r.FormValue("username"))
	userKey := "login:user:" + username
	if normalizeErr == nil {
		if ok, wait := a.limiter.Allow(userKey, loginUserPolicy); !ok {
			a.tooMany(w, r, "login", data, wait)
			return
		}
	}

	passwordHash := a.dummyPasswordHash
	var userID string
	var onboarding bool
	if normalizeErr == nil {
		user, err := a.store.UserByUsername(r.Context(), username)
		if err == nil {
			passwordHash = user.PasswordHash
			userID = user.ID
			onboarding = user.OnboardingCompleted
		} else if !errors.Is(err, store.ErrNotFound) {
			a.logger.Error("error consultando usuario", "error", err)
			http.Error(w, "No se pudo completar el inicio de sesión.", http.StatusInternalServerError)
			return
		}
	}

	valid, verifyErr := auth.VerifyPassword(passwordHash, r.FormValue("password"))
	if verifyErr != nil {
		a.logger.Error("hash de contraseña inválido", "error", verifyErr)
		valid = false
	}
	if !valid || userID == "" {
		_ = a.store.Audit(r.Context(), "", "login", "fallido", ip)
		data.Error = "Usuario o contraseña incorrectos."
		data = a.csrfData(w, r, data)
		a.render(w, http.StatusUnauthorized, "login", data)
		return
	}

	// Authentication changes browser state. Prevent an intermediary from
	// caching/replaying the redirect response that carries Set-Cookie.
	setAuthNoStore(w)
	if err := a.createLoginSession(w, r, userID); err != nil {
		a.logger.Error("no se pudo crear sesión", "error", err)
		http.Error(w, "No se pudo completar el inicio de sesión.", http.StatusInternalServerError)
		return
	}
	a.limiter.Reset(userKey)
	a.limiter.Reset(ipKey)
	_ = a.store.Audit(r.Context(), userID, "login", "correcto", ip)
	if onboarding {
		http.Redirect(w, r, "/inicio", http.StatusFound)
	} else {
		http.Redirect(w, r, "/bienvenida", http.StatusFound)
	}
}

func (a *App) logoutPost(w http.ResponseWriter, r *http.Request) {
	setAuthNoStore(w)
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	a.clearLoginSession(w, r)
	if user != nil {
		_ = a.store.Audit(r.Context(), user.ID, "logout", "correcto", a.clientIP(r))
	}
	http.Redirect(w, r, "/iniciar-sesion", http.StatusSeeOther)
}

func (a *App) onboardingGet(w http.ResponseWriter, r *http.Request) {
	setAuthNoStore(w)
	user := userFromContext(r.Context())
	if user.OnboardingCompleted {
		http.Redirect(w, r, "/inicio", http.StatusSeeOther)
		return
	}
	data := a.csrfData(w, r, pageData{Title: "Bienvenido", Description: "Conoce cómo funcionará tu almacenamiento unificado.", CurrentPath: "/bienvenida", User: user})
	a.render(w, http.StatusOK, "onboarding", data)
}

func (a *App) onboardingCompletePost(w http.ResponseWriter, r *http.Request) {
	if !a.parseProtectedForm(w, r) {
		return
	}
	user := userFromContext(r.Context())
	if err := a.store.CompleteOnboarding(r.Context(), user.ID); err != nil {
		http.Error(w, "No se pudo completar la bienvenida.", http.StatusInternalServerError)
		return
	}
	_ = a.store.Audit(r.Context(), user.ID, "onboarding", "completado", a.clientIP(r))
	http.Redirect(w, r, "/inicio", http.StatusSeeOther)
}

func (a *App) parseProtectedForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Formulario inválido.", http.StatusBadRequest)
		return false
	}
	if !a.validCSRF(r) {
		http.Error(w, "La sesión del formulario no es válida. Recarga la página e inténtalo de nuevo.", http.StatusBadRequest)
		return false
	}
	return true
}
