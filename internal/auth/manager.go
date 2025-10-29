package auth

import (
	"context"
	"errors"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"miaomiaowu/internal/storage"
)

type Credentials struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type Manager struct {
	repo     *storage.TrafficRepository
	mu       sync.RWMutex
	username string
}

func NewManager(repo *storage.TrafficRepository) (*Manager, error) {
	if repo == nil {
		return nil, errors.New("auth manager requires repository")
	}

	m := &Manager{repo: repo}
	return m, nil
}

func (m *Manager) Authenticate(ctx context.Context, username, password string) (bool, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return false, nil
	}

	user, err := m.repo.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return false, nil
		}
		return false, err
	}

	if !user.IsActive {
		return false, nil
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return false, nil
	}

	return true, nil
}

func (m *Manager) Update(ctx context.Context, username, password string) error {
	if username == "" && password == "" {
		return errors.New("username or password must be provided")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.username

	if username != "" && username != current {
		if err := m.repo.RenameUser(ctx, current, username); err != nil {
			return err
		}
		current = username
	}

	if password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := m.repo.UpdateUserPassword(ctx, current, string(hash)); err != nil {
			return err
		}
	}

	m.username = current
	return nil
}

func (m *Manager) ChangePassword(ctx context.Context, username, currentPassword, newPassword string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if currentPassword == "" || newPassword == "" {
		return errors.New("passwords are required")
	}

	user, err := m.repo.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return errors.New("user not found")
		}
		return err
	}

	if !user.IsActive {
		return errors.New("user is disabled")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(currentPassword)) != nil {
		return errors.New("current password is incorrect")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := m.repo.UpdateUserPassword(ctx, username, string(hash)); err != nil {
		return err
	}

	if username == m.username {
		m.mu.Lock()
		m.username = username
		m.mu.Unlock()
	}

	return nil
}

func (m *Manager) Credentials(ctx context.Context) (Credentials, error) {
	m.mu.RLock()
	username := m.username
	m.mu.RUnlock()

	user, err := m.repo.GetUser(ctx, username)
	if err != nil {
		return Credentials{}, err
	}

	return Credentials{Username: user.Username, PasswordHash: user.PasswordHash}, nil
}

// User retrieves the stored user record for the provided username.
func (m *Manager) User(ctx context.Context, username string) (storage.User, error) {
	return m.repo.GetUser(ctx, username)
}

// ValidatePassword checks if the provided password is correct for the given user.
func (m *Manager) ValidatePassword(ctx context.Context, username, password string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}
	if password == "" {
		return errors.New("password is required")
	}

	user, err := m.repo.GetUser(ctx, username)
	if err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return errors.New("user not found")
		}
		return err
	}

	if !user.IsActive {
		return errors.New("user is disabled")
	}

	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return errors.New("password is incorrect")
	}

	return nil
}
