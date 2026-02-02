package repo

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

type UserRepository struct {
	db *database.DB
}

func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, user *domain.User) error {
	user.ID = uuid.New().String()
	user.PublicID = generatePublicID()
	user.PublicStatuses = []string{"watching", "completed", "plan_to_watch"}
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()

	query := `
		INSERT INTO users (id, username, password_hash, telegram_id, public_id, public_statuses, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Username, user.PasswordHash, user.TelegramID,
		user.PublicID, pq.Array(user.PublicStatuses),
		user.Role, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return errors.AlreadyExists("username")
		}
		return fmt.Errorf("create user: %w", err)
	}

	return nil
}

func generatePublicID() string {
	return fmt.Sprintf("user%d", rand.Intn(9000000)+1000000)
}

func (r *UserRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	query := `
		SELECT id, username, password_hash, telegram_id, public_id, public_statuses, role, created_at, updated_at
		FROM users
		WHERE id = $1 AND deleted_at IS NULL
	`

	var user domain.User
	var publicStatuses pq.StringArray
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.TelegramID,
		&user.PublicID, &publicStatuses, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	user.PublicStatuses = publicStatuses

	return &user, nil
}

func (r *UserRepository) GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	query := `
		SELECT id, username, password_hash, telegram_id, public_id, public_statuses, role, created_at, updated_at
		FROM users
		WHERE telegram_id = $1 AND deleted_at IS NULL
	`

	var user domain.User
	var publicStatuses pq.StringArray
	err := r.db.QueryRowContext(ctx, query, telegramID).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.TelegramID,
		&user.PublicID, &publicStatuses, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Not found is not an error for this use case
		}
		return nil, fmt.Errorf("get user by telegram id: %w", err)
	}
	user.PublicStatuses = publicStatuses

	return &user, nil
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `
		SELECT id, username, password_hash, telegram_id, public_id, public_statuses, role, created_at, updated_at
		FROM users
		WHERE username = $1 AND deleted_at IS NULL
	`

	var user domain.User
	var publicStatuses pq.StringArray
	err := r.db.QueryRowContext(ctx, query, username).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.TelegramID,
		&user.PublicID, &publicStatuses, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by username: %w", err)
	}
	user.PublicStatuses = publicStatuses

	return &user, nil
}

func (r *UserRepository) Update(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now()

	query := `
		UPDATE users
		SET username = $1, password_hash = $2, telegram_id = $3, public_id = $4, public_statuses = $5, role = $6, updated_at = $7
		WHERE id = $8 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query,
		user.Username, user.PasswordHash, user.TelegramID,
		user.PublicID, pq.Array(user.PublicStatuses),
		user.Role, user.UpdatedAt, user.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return errors.AlreadyExists("username or public_id")
		}
		return fmt.Errorf("update user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update user rows affected: %w", err)
	}
	if rows == 0 {
		return errors.NotFound("user")
	}

	return nil
}

func (r *UserRepository) GetByPublicID(ctx context.Context, publicID string) (*domain.User, error) {
	query := `
		SELECT id, username, password_hash, telegram_id, public_id, public_statuses, role, created_at, updated_at
		FROM users
		WHERE public_id = $1 AND deleted_at IS NULL
	`

	var user domain.User
	var publicStatuses pq.StringArray
	err := r.db.QueryRowContext(ctx, query, publicID).Scan(
		&user.ID, &user.Username, &user.PasswordHash, &user.TelegramID,
		&user.PublicID, &publicStatuses, &user.Role, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.NotFound("user")
		}
		return nil, fmt.Errorf("get user by public_id: %w", err)
	}
	user.PublicStatuses = publicStatuses

	return &user, nil
}

func (r *UserRepository) UpdatePublicID(ctx context.Context, userID, publicID string) error {
	query := `
		UPDATE users
		SET public_id = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, publicID, time.Now(), userID)
	if err != nil {
		if isUniqueViolation(err) {
			return errors.AlreadyExists("public_id")
		}
		return fmt.Errorf("update public_id: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update public_id rows affected: %w", err)
	}
	if rows == 0 {
		return errors.NotFound("user")
	}

	return nil
}

func (r *UserRepository) UpdatePublicStatuses(ctx context.Context, userID string, statuses []string) error {
	query := `
		UPDATE users
		SET public_statuses = $1, updated_at = $2
		WHERE id = $3 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, pq.Array(statuses), time.Now(), userID)
	if err != nil {
		return fmt.Errorf("update public_statuses: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update public_statuses rows affected: %w", err)
	}
	if rows == 0 {
		return errors.NotFound("user")
	}

	return nil
}

func (r *UserRepository) ExistsByPublicID(ctx context.Context, publicID string) (bool, error) {
	query := `
		SELECT EXISTS(SELECT 1 FROM users WHERE public_id = $1 AND deleted_at IS NULL)
	`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, publicID)
	if err != nil {
		return false, fmt.Errorf("check public_id exists: %w", err)
	}

	return exists, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	query := `
		UPDATE users
		SET deleted_at = $1
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete user rows affected: %w", err)
	}
	if rows == 0 {
		return errors.NotFound("user")
	}

	return nil
}

func (r *UserRepository) ExistsByUsername(ctx context.Context, username string) (bool, error) {
	query := `
		SELECT EXISTS(SELECT 1 FROM users WHERE username = $1 AND deleted_at IS NULL)
	`

	var exists bool
	err := r.db.GetContext(ctx, &exists, query, username)
	if err != nil {
		return false, fmt.Errorf("check username exists: %w", err)
	}

	return exists, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (
		// PostgreSQL unique violation
		contains(err.Error(), "unique constraint") ||
		contains(err.Error(), "duplicate key"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
