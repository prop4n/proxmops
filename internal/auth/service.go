package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/prop4n/proxmops/internal/store"
)

// Session lifetime.
const sessionTTL = 7 * 24 * time.Hour

// Errors returned by the service.
var (
	ErrSetupClosed        = errors.New("auth: setup already completed")
	ErrInvalidBootstrap   = errors.New("auth: invalid setup token")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
)

// Repository is the persistence the service needs.
type Repository interface {
	CountAccounts(ctx context.Context) (int, error)
	CreateAccount(ctx context.Context, username, passwordHash string) (store.Account, error)
	AccountByUsername(ctx context.Context, username string) (store.Account, error)
	AccountByID(ctx context.Context, id int64) (store.Account, error)
	CreateSession(ctx context.Context, tokenHash string, accountID int64, expiresAt time.Time) error
	Session(ctx context.Context, tokenHash string) (store.Session, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

// Service implements account setup, login, and session validation.
type Service struct {
	repo Repository
	log  *slog.Logger

	mu            sync.Mutex
	bootstrapHash []byte // sha256 of the setup token; nil once setup is closed
}

// New returns a Service. Call Init once at startup to arm the bootstrap token.
func New(repo Repository, log *slog.Logger) *Service {
	return &Service{repo: repo, log: log}
}

// Init generates and logs a one-time setup token when no account exists yet.
// It is a no-op once at least one account is present.
func (s *Service) Init(ctx context.Context) error {
	n, err := s.repo.CountAccounts(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}

	token, err := randomToken()
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(token))

	s.mu.Lock()
	s.bootstrapHash = sum[:]
	s.mu.Unlock()

	s.log.Info("no account yet: create the first admin with this one-time setup token",
		"setupToken", token)
	return nil
}

// NeedsSetup reports whether no account exists yet.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.repo.CountAccounts(ctx)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// Setup creates the first admin account after validating the setup token, and
// returns a session token for the new account.
func (s *Service) Setup(ctx context.Context, setupToken, username, password string) (string, error) {
	n, err := s.repo.CountAccounts(ctx)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return "", ErrSetupClosed
	}

	s.mu.Lock()
	valid := s.bootstrapHash != nil && matchToken(s.bootstrapHash, setupToken)
	s.mu.Unlock()
	if !valid {
		return "", ErrInvalidBootstrap
	}

	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	account, err := s.repo.CreateAccount(ctx, username, hash)
	if err != nil {
		return "", err
	}

	// Setup is now closed: retire the bootstrap token.
	s.mu.Lock()
	s.bootstrapHash = nil
	s.mu.Unlock()

	return s.newSession(ctx, account.ID)
}

// Login validates credentials and returns a session token.
func (s *Service) Login(ctx context.Context, username, password string) (string, error) {
	account, err := s.repo.AccountByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}
	ok, err := VerifyPassword(account.PasswordHash, password)
	if err != nil || !ok {
		return "", ErrInvalidCredentials
	}
	return s.newSession(ctx, account.ID)
}

// Authenticate resolves a session token to its account.
func (s *Service) Authenticate(ctx context.Context, sessionToken string) (store.Account, error) {
	sess, err := s.repo.Session(ctx, hashToken(sessionToken))
	if errors.Is(err, store.ErrNotFound) {
		return store.Account{}, ErrInvalidCredentials
	}
	if err != nil {
		return store.Account{}, err
	}
	if time.Now().After(sess.ExpiresAt) {
		_ = s.repo.DeleteSession(ctx, sess.TokenHash)
		return store.Account{}, ErrInvalidCredentials
	}
	return s.repo.AccountByID(ctx, sess.AccountID)
}

// Logout invalidates a session token.
func (s *Service) Logout(ctx context.Context, sessionToken string) error {
	return s.repo.DeleteSession(ctx, hashToken(sessionToken))
}

// SessionTTL is the lifetime granted to a new session.
func (s *Service) SessionTTL() time.Duration { return sessionTTL }

// newSession creates and stores a session, returning the plaintext token.
func (s *Service) newSession(ctx context.Context, accountID int64) (string, error) {
	token, err := randomToken()
	if err != nil {
		return "", err
	}
	if err := s.repo.CreateSession(ctx, hashToken(token), accountID, time.Now().Add(sessionTTL)); err != nil {
		return "", err
	}
	return token, nil
}

// randomToken returns a URL-safe 256-bit random token.
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashToken returns the hex-encoded sha256 of a token, used as its DB key.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// matchToken compares a token against a stored sha256 in constant time.
func matchToken(wantHash []byte, token string) bool {
	sum := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(wantHash, sum[:]) == 1
}
