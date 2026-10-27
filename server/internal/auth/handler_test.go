package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"calcqueue/internal/openapi"
)

type fakeCredential struct {
	user User
	hash string
}
type fakeSession struct {
	user    User
	session session
}
type fakeStore struct {
	mu       sync.Mutex
	users    map[string]fakeCredential
	sessions map[[sha256.Size]byte]fakeSession
	err      error
}

var (
	_ userStore    = (*fakeStore)(nil)
	_ sessionStore = (*fakeStore)(nil)
)

func (f *fakeStore) createUserSession(_ context.Context, login, hash string, sess session) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return User{}, f.err
	}
	if _, ok := f.users[login]; ok {
		return User{}, ErrLoginTaken
	}
	user := User{ID: uuid.New(), Login: login, CreatedAt: testNow}
	f.users[login] = fakeCredential{user: user, hash: hash}
	f.sessions[sess.key] = fakeSession{user: user, session: sess}
	return user, nil
}

func (f *fakeStore) credentials(_ context.Context, login string) (User, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return User{}, "", f.err
	}
	record, ok := f.users[login]
	if !ok {
		return User{}, "", ErrInvalidCredentials
	}
	return record.user, record.hash, nil
}

func (f *fakeStore) createSession(_ context.Context, user User, sess session) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sessions[sess.key] = fakeSession{user: user, session: sess}
	return nil
}

func (f *fakeStore) sessionUser(_ context.Context, key [sha256.Size]byte, now time.Time) (User, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return User{}, f.err
	}
	record, ok := f.sessions[key]
	if !ok || !record.session.expiresAt.After(now) {
		return User{}, ErrUnauthorized
	}
	return record.user, nil
}

func (f *fakeStore) deleteSession(_ context.Context, key [sha256.Size]byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	delete(f.sessions, key)
	return nil
}

var testNow = time.Date(2026, time.October, 27, 12, 0, 0, 0, time.UTC)

func newTestHandler() (*Handler, *fakeStore) {
	store := &fakeStore{users: make(map[string]fakeCredential), sessions: make(map[[sha256.Size]byte]fakeSession)}
	service := newService(store, store, 90*time.Minute)
	service.bcryptCost = bcrypt.MinCost
	service.now = func() time.Time { return testNow }
	return &Handler{service: service}, store
}

func request(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(t.Context())
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func sessionResponse(t *testing.T, w *httptest.ResponseRecorder, status int) openapi.AuthSession {
	t.Helper()
	if w.Code != status {
		t.Fatalf("auth status = %d, want %d; body %s", w.Code, status, w.Body.String())
	}
	var result openapi.AuthSession
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode session fields: %v", err)
	}
	if len(fields) != 3 {
		t.Errorf("session fields = %v, want token/expires_at/user only", fields)
	}
	var userFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["user"], &userFields); err != nil {
		t.Fatalf("decode user fields: %v", err)
	}
	if len(userFields) != 3 {
		t.Errorf("user fields = %v, want id/login/created_at only", userFields)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(result.Token)
	if err != nil || len(raw) != 32 {
		t.Errorf("token decodes to %d bytes, error = %v, want 32 bytes", len(raw), err)
	}
	if result.User.Id == uuid.Nil || result.User.CreatedAt.IsZero() {
		t.Errorf("session user = %+v, want ID and created_at", result.User)
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Errorf("content type = %q, want JSON", w.Header().Get("Content-Type"))
	}
	return result
}

func assertError(t *testing.T, w *httptest.ResponseRecorder, status int, code, message string) {
	t.Helper()
	if w.Code != status {
		t.Errorf("status = %d, want %d; body %s", w.Code, status, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	want := map[string]any{"error": map[string]any{"code": code, "message": message}}
	if diff := cmp.Diff(want, body); diff != "" {
		t.Errorf("error envelope (-want +got):\n%s", diff)
	}
}

func TestHandlerSessionLifecycle(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	const body = `{"login":"alice","password":"пароль12"}`
	registered := sessionResponse(t, request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", body, ""), 201)
	if registered.User.Login != "alice" {
		t.Errorf("login = %q, want alice", registered.User.Login)
	}
	if !registered.ExpiresAt.Equal(testNow.Add(h.service.ttl)) {
		t.Errorf("expiry = %v, want %v", registered.ExpiresAt, testNow.Add(h.service.ttl))
	}
	hash := store.users["alice"].hash
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("пароль12")); err != nil {
		t.Errorf("stored bcrypt password: %v", err)
	}
	assertError(t, request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", body, ""), 409, "login_taken", "логин занят")
	loggedIn := sessionResponse(t, request(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login", body, ""), 200)
	if loggedIn.Token == registered.Token {
		t.Error("login reused registration token")
	}
	if diff := cmp.Diff(registered.User, loggedIn.User); diff != "" {
		t.Errorf("login user (-want +got):\n%s", diff)
	}
	for _, body := range []string{`{"login":"alice","password":"wrongpass"}`, `{"login":"missing","password":"wrongpass"}`} {
		assertError(t, request(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login", body, ""), 401, "invalid_credentials", "неверный логин или пароль")
	}
	me := h.RequireAuth(http.HandlerFunc(h.Me))
	w := request(t, me, "GET", "/api/me", "", registered.Token)
	if w.Code != 200 {
		t.Fatalf("me status = %d, want 200", w.Code)
	}
	var user openapi.User
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if diff := cmp.Diff(registered.User, user); diff != "" {
		t.Errorf("me user (-want +got):\n%s", diff)
	}
	logout := h.RequireAuth(http.HandlerFunc(h.Logout))
	w = request(t, logout, "POST", "/api/auth/logout", "", registered.Token)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Errorf("logout status/body = %d/%q, want 204/empty", w.Code, w.Body.String())
	}
	assertError(t, request(t, me, "GET", "/api/me", "", registered.Token), 401, "unauthorized", "требуется вход")
	assertError(t, request(t, logout, "POST", "/api/auth/logout", "", registered.Token), 401, "unauthorized", "требуется вход")
	if w := request(t, me, "GET", "/api/me", "", loggedIn.Token); w.Code != 200 {
		t.Errorf("other session after logout = %d, want 200", w.Code)
	}
	h.service.now = func() time.Time { return loggedIn.ExpiresAt }
	assertError(t, request(t, me, "GET", "/api/me", "", loggedIn.Token), 401, "unauthorized", "требуется вход")
}

func TestHandlerValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, login, password string
		valid                 bool
	}{
		{name: "minimum", login: "a_0", password: "12345678", valid: true},
		{name: "maximum", login: strings.Repeat("a", 32), password: strings.Repeat("p", 72), valid: true},
		{name: "four Cyrillic letters", login: "abcd", password: "абвг", valid: true},
		{name: "36 Cyrillic letters", login: "abcd", password: strings.Repeat("я", 36), valid: true},
		{name: "37 Cyrillic letters", login: "abcd", password: strings.Repeat("я", 37)},
		{name: "short password", login: "abcd", password: "1234567"},
		{name: "long password", login: "abcd", password: strings.Repeat("a", 73)},
		{name: "short login", login: "ab", password: "12345678"},
		{name: "long login", login: strings.Repeat("a", 33), password: "12345678"},
		{name: "uppercase", login: "Alice", password: "12345678"},
		{name: "leading space", login: " alice", password: "12345678"},
		{name: "trailing space", login: "alice ", password: "12345678"},
		{name: "Cyrillic login", login: "алиса", password: "12345678"},
		{name: "punctuation", login: "ali.ce", password: "12345678"},
		{name: "newline", login: "alice\n", password: "12345678"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler()
			body, err := json.Marshal(map[string]string{"login": tc.login, "password": tc.password})
			if err != nil {
				t.Fatal(err)
			}
			for _, handler := range []http.HandlerFunc{h.Register, h.Login} {
				w := request(t, handler, "POST", "/", string(body), "")
				if !tc.valid {
					assertError(t, w, 422, "validation_error", "проверьте поля формы")
					continue
				}
				if w.Code != 201 && w.Code != 200 {
					t.Errorf("valid credentials status = %d, want 200/201; body %s", w.Code, w.Body.String())
				}
			}
		})
	}
}

func TestHandlerInvalidJSON(t *testing.T) {
	t.Parallel()
	bodies := []string{
		``, `{`, `null`, `[]`, `{}`, `{"login":"alice"}`, `{"password":"12345678"}`,
		`{"login":null,"password":"12345678"}`, `{"login":"alice","password":null}`,
		`{"login":123,"password":"12345678"}`, `{"login":"alice","password":123}`,
		`{"login":"alice","password":"12345678","extra":true}`,
		`{"login":"alice","password":"12345678"} {}`,
		`{"login":"alice","password":"12345678","Password":null}`,
		`{"login":"alice","password":"` + strings.Repeat("a", 16<<10) + `"}`,
	}
	for i, body := range bodies {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler()
			for _, handler := range []http.HandlerFunc{h.Register, h.Login} {
				assertError(t, request(t, handler, "POST", "/", body, ""), 422, "validation_error", "некорректное тело запроса")
			}
		})
	}
}

func TestRequireAuthAndContext(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler()
	result, err := h.service.register(t.Context(), "alice", "12345678")
	if err != nil {
		t.Fatal(err)
	}
	authenticated := false
	protected := h.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authenticated = true
		user, ok := UserFromContext(r.Context())
		if !ok || user != result.user {
			t.Errorf("context user = %+v/%v, want %+v/true", user, ok, result.user)
		}
		w.WriteHeader(200)
	}))
	headers := []string{"", "Basic abc", "Bearer", "Bearer invalid", "Bearer " + strings.Repeat("a", 43), "Bearer " + result.token + "=", "Bearer " + result.token + " ", "Bearer  " + result.token}
	for _, header := range headers {
		r := httptest.NewRequest("GET", "/api/me", nil).WithContext(t.Context())
		r.Header.Set("Authorization", header)
		w := httptest.NewRecorder()
		protected.ServeHTTP(w, r)
		assertError(t, w, 401, "unauthorized", "требуется вход")
	}
	if authenticated {
		t.Error("invalid token called protected handler")
	}
	r := httptest.NewRequest("GET", "/api/me", nil).WithContext(t.Context())
	r.Header.Add("Authorization", "Bearer "+result.token)
	r.Header.Add("Authorization", "Bearer "+result.token)
	w := httptest.NewRecorder()
	protected.ServeHTTP(w, r)
	assertError(t, w, 401, "unauthorized", "требуется вход")
	if w := request(t, protected, "GET", "/api/me", "", result.token); w.Code != 200 {
		t.Errorf("valid token = %d, want 200", w.Code)
	}
	if !authenticated {
		t.Error("valid token did not call protected handler")
	}
	// A different typed key cannot impersonate the private key.
	type foreignContextKey struct{}
	ctx := context.WithValue(t.Context(), foreignContextKey{}, result.user)
	if user, ok := UserFromContext(ctx); ok || user != (User{}) {
		t.Errorf("unauthenticated context user = %+v/%v", user, ok)
	}
	for _, handler := range []http.HandlerFunc{h.Me, h.Logout} {
		assertError(t, request(t, handler, "POST", "/", "", ""), 401, "unauthorized", "требуется вход")
	}
}

func TestHandlerInternalErrors(t *testing.T) {
	t.Parallel()
	h, store := newTestHandler()
	result, err := h.service.register(t.Context(), "alice", "12345678")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(t.Context(), sessionContextKey{}, sha256.Sum256([]byte(result.token)))
	store.err = errors.New("database unavailable with secret details")
	handlers := []http.Handler{http.HandlerFunc(h.Register), http.HandlerFunc(h.Login), h.RequireAuth(http.HandlerFunc(h.Me))}
	for _, handler := range handlers {
		assertError(t, request(t, handler, "POST", "/", `{"login":"alice","password":"12345678"}`, result.token), 500, "internal_error", "внутренняя ошибка сервера")
	}
	w := httptest.NewRecorder()
	h.Logout(w, httptest.NewRequest("POST", "/", nil).WithContext(ctx))
	assertError(t, w, 500, "internal_error", "внутренняя ошибка сервера")
}
