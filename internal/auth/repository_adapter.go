package auth

import (
	"context"
	"miaomiaowu/internal/storage"
)

// RepositoryAdapter adapts storage.TrafficRepository to implement UserRepository interface.
type RepositoryAdapter struct {
	repo *storage.TrafficRepository
}

// NewRepositoryAdapter creates a new adapter for the traffic repository.
func NewRepositoryAdapter(repo *storage.TrafficRepository) UserRepository {
	return &RepositoryAdapter{repo: repo}
}

// GetUser retrieves user information from the storage repository.
func (a *RepositoryAdapter) GetUser(ctx context.Context, username string) (User, error) {
	storageUser, err := a.repo.GetUser(ctx, username)
	if err != nil {
		return User{}, err
	}

	return User{
		Username: storageUser.Username,
		Role:     storageUser.Role,
		IsActive: storageUser.IsActive,
	}, nil
}
