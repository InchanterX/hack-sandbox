//go:build integration

package auth

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"calcqueue/internal/openapi"
	"calcqueue/internal/testdb"
	"calcqueue/migrations"
)

func newIntegrationHandler(t *testing.T) (*Handler, *pgxpool.Pool) {
	t.Helper()
	pool, _ := testdb.NewSchema(t)
	if err := migrations.Apply(t.Context(), pool); err != nil {
		t.Fatalf("apply auth schema: %v", err)
	}
	h := NewHandler(pool, 90*time.Minute)
	h.service.bcryptCost = bcrypt.MinCost
	h.service.now = func() time.Time { return testNow }
	return h, pool
}

func TestStoreHTTPAuthentication(t *testing.T) {
	h, pool := newIntegrationHandler(t)
	const body = `{"login":"alice","password":"абвг"}`
	registered := sessionResponse(t, request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", body, ""), 201)
	var hash string
	if err := pool.QueryRow(t.Context(), `SELECT password_hash FROM users WHERE id = $1`, registered.User.Id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("абвг")); err != nil {
		t.Errorf("database bcrypt password: %v", err)
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if err != nil || cost != bcrypt.MinCost {
		t.Errorf("bcrypt cost/error = %d/%v, want %d/nil", cost, err, bcrypt.MinCost)
	}
	var key []byte
	var expiresAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT token_sha256, expires_at FROM sessions WHERE user_id = $1`, registered.User.Id).Scan(&key, &expiresAt); err != nil {
		t.Fatal(err)
	}
	wantKey := sha256.Sum256([]byte(registered.Token))
	if diff := cmp.Diff(wantKey[:], key); diff != "" {
		t.Errorf("stored SHA-256 (-want +got):\n%s", diff)
	}
	if !expiresAt.Equal(registered.ExpiresAt) || !expiresAt.Equal(testNow.Add(90*time.Minute)) {
		t.Errorf("database/API expiry = %v/%v, want %v", expiresAt, registered.ExpiresAt, testNow.Add(90*time.Minute))
	}
	assertError(t, request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", body, ""), 409, "login_taken", "логин занят")
	loggedIn := sessionResponse(t, request(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login", body, ""), 200)
	if loggedIn.Token == registered.Token {
		t.Error("database login reused token")
	}
	if diff := cmp.Diff(registered.User, loggedIn.User); diff != "" {
		t.Errorf("database login user (-want +got):\n%s", diff)
	}
	for _, body := range []string{`{"login":"alice","password":"incorrect"}`, `{"login":"unknown","password":"incorrect"}`} {
		assertError(t, request(t, http.HandlerFunc(h.Login), "POST", "/api/auth/login", body, ""), 401, "invalid_credentials", "неверный логин или пароль")
	}
	other := sessionResponse(t, request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", `{"login":"other","password":"12345678"}`, ""), 201)
	me := h.RequireAuth(http.HandlerFunc(h.Me))
	for _, token := range []string{registered.Token, loggedIn.Token, other.Token} {
		w := request(t, me, "GET", "/api/me", "", token)
		if w.Code != 200 {
			t.Errorf("database me status = %d, want 200", w.Code)
		}
	}
	w := request(t, h.RequireAuth(http.HandlerFunc(h.Logout)), "POST", "/api/auth/logout", "", registered.Token)
	if w.Code != 204 || w.Body.Len() != 0 {
		t.Errorf("database logout = %d/%q, want 204/empty", w.Code, w.Body.String())
	}
	assertError(t, request(t, me, "GET", "/api/me", "", registered.Token), 401, "unauthorized", "требуется вход")
	for _, token := range []string{loggedIn.Token, other.Token} {
		if w := request(t, me, "GET", "/api/me", "", token); w.Code != 200 {
			t.Errorf("other session after logout = %d, want 200", w.Code)
		}
	}
	loginKey := sha256.Sum256([]byte(loggedIn.Token))
	if _, err := pool.Exec(t.Context(), `UPDATE sessions SET expires_at = $1 WHERE token_sha256 = $2`, testNow, loginKey[:]); err != nil {
		t.Fatal(err)
	}
	assertError(t, request(t, me, "GET", "/api/me", "", loggedIn.Token), 401, "unauthorized", "требуется вход")
	assertError(t, request(t, me, "GET", "/api/me", "", ""), 401, "unauthorized", "требуется вход")
	assertError(t, request(t, me, "GET", "/api/me", "", "invalid"), 401, "unauthorized", "требуется вход")
}

func TestStoreConcurrentRegistration(t *testing.T) {
	h, pool := newIntegrationHandler(t)
	const attempts = 8
	start := make(chan struct{})
	// Each worker sends exactly once; capacity lets all workers finish before collection.
	results := make(chan *httptest.ResponseRecorder, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			<-start
			w := request(t, http.HandlerFunc(h.Register), "POST", "/api/auth/register", `{"login":"racing","password":"12345678"}`, "")
			results <- w
		})
	}
	close(start)
	wg.Wait()
	close(results)
	counts := make(map[int]int)
	for w := range results {
		counts[w.Code]++
		if w.Code == 409 {
			assertError(t, w, 409, "login_taken", "логин занят")
		}
	}
	if diff := cmp.Diff(map[int]int{201: 1, 409: attempts - 1}, counts); diff != "" {
		t.Errorf("concurrent registration status counts (-want +got):\n%s", diff)
	}
	var users, sessions int
	if err := pool.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM users), (SELECT count(*) FROM sessions)`).Scan(&users, &sessions); err != nil {
		t.Fatal(err)
	}
	if users != 1 || sessions != 1 {
		t.Errorf("concurrent database users/sessions = %d/%d, want 1/1", users, sessions)
	}
}

func TestStoreRegistrationRollback(t *testing.T) {
	_, pool := newIntegrationHandler(t)
	store := &store{pool: pool}
	// Two users cannot share a session key. Failure of the second insert must
	// roll back its user too, rather than leaving an account without a session.
	sess := session{key: sha256.Sum256([]byte("collision")), expiresAt: testNow.Add(time.Hour)}
	if _, err := store.createUserSession(t.Context(), "first", "hash", sess); err != nil {
		t.Fatal(err)
	}
	if _, err := store.createUserSession(t.Context(), "second", "hash", sess); err == nil || errors.Is(err, ErrLoginTaken) {
		t.Errorf("session collision error = %v, want infrastructure error", err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM users WHERE login = 'second'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("users after failed registration = %d, want 0", count)
	}
}

func TestStoreSessionExpiryBoundary(t *testing.T) {
	h, _ := newIntegrationHandler(t)
	result, err := h.service.register(t.Context(), "alice", "12345678")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		now  time.Time
		want int
	}{
		{name: "before expiry", now: result.expiresAt.Add(-time.Microsecond), want: 200},
		{name: "at expiry", now: result.expiresAt, want: 401},
		{name: "after expiry", now: result.expiresAt.Add(time.Microsecond), want: 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.service.now = func() time.Time { return tc.now }
			w := request(t, h.RequireAuth(http.HandlerFunc(h.Me)), "GET", "/api/me", "", result.token)
			if tc.want == 401 {
				assertError(t, w, 401, "unauthorized", "требуется вход")
				return
			}
			if w.Code != tc.want {
				t.Errorf("expiry boundary status = %d, want %d", w.Code, tc.want)
			}
			var user openapi.User
			if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(wireUser(result.user), user); diff != "" {
				t.Errorf("authenticated user (-want +got):\n%s", diff)
			}
		})
	}
}
