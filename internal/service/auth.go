package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/hddq/podgist/internal/domain"
	"github.com/hddq/podgist/internal/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const SessionLifetime = 365 * 24 * time.Hour

type AuthService struct {
	store      *store.Store
	bcryptCost int
}

func NewAuthService(s *store.Store, bcryptCost int) *AuthService {
	return &AuthService{store: s, bcryptCost: bcryptCost}
}

func (s *AuthService) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid credentials")
		}
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

func (s *AuthService) GetUserBySessionID(ctx context.Context, sessionID string) (*domain.User, error) {
	if sessionID == "" {
		return nil, errors.New("missing session")
	}
	now := time.Now().UTC()
	if err := s.store.DeleteExpiredSessions(ctx, now); err != nil {
		return nil, err
	}
	user, err := s.store.GetUserBySessionID(ctx, sessionID, now)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid session")
		}
		return nil, err
	}
	return user, nil
}

func (s *AuthService) CreateSession(ctx context.Context, userID int64) (*domain.Session, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	sessionID := base64.RawURLEncoding.EncodeToString(token)
	expiresAt := time.Now().UTC().Add(SessionLifetime)
	return s.store.CreateSession(ctx, sessionID, userID, expiresAt)
}

func (s *AuthService) RefreshSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	expiresAt := time.Now().UTC().Add(SessionLifetime)
	if err := s.store.TouchSession(ctx, sessionID, expiresAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("invalid session")
		}
		return nil, err
	}
	return &domain.Session{ID: sessionID, ExpiresAt: expiresAt}, nil
}

func (s *AuthService) DeleteSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, sessionID)
}

func (s *AuthService) CreateUser(ctx context.Context, username, password string) (*domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, err
	}
	return s.store.CreateUser(ctx, username, string(hash))
}
