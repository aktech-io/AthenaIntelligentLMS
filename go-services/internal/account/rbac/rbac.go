// Package rbac is the account-service's store for the global role->permission
// matrix. The matrix is the single source of truth for authorization: effective
// permissions are resolved here, stamped into the JWT at login, and enforced
// locally by every service via auth.RequirePermission.
package rbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Permission is one entry in the catalog of gated operations.
type Permission struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// Matrix is the full role->permission mapping plus the permission catalog and
// the current version (bumped on every change).
type Matrix struct {
	Version     int64               `json:"version"`
	Permissions []Permission        `json:"permissions"`     // catalog
	Roles       map[string][]string `json:"roles"`           // role -> permission keys
}

// Store reads and mutates the matrix.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore creates a Store over the given pool.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Version returns the current matrix version.
func (s *Store) Version(ctx context.Context) (int64, error) {
	var v int64
	err := s.pool.QueryRow(ctx, `SELECT version FROM rbac_meta WHERE singleton`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("rbac version: %w", err)
	}
	return v, nil
}

// ListPermissions returns the permission catalog ordered by category then key.
func (s *Store) ListPermissions(ctx context.Context) ([]Permission, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT key, description, category FROM rbac_permissions ORDER BY category, key`)
	if err != nil {
		return nil, fmt.Errorf("list permissions: %w", err)
	}
	defer rows.Close()

	var out []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.Key, &p.Description, &p.Category); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Matrix returns the full matrix: catalog + role grants + version.
func (s *Store) Matrix(ctx context.Context) (*Matrix, error) {
	perms, err := s.ListPermissions(ctx)
	if err != nil {
		return nil, err
	}
	version, err := s.Version(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx,
		`SELECT role, permission_key FROM rbac_role_permissions ORDER BY role, permission_key`)
	if err != nil {
		return nil, fmt.Errorf("matrix grants: %w", err)
	}
	defer rows.Close()

	roles := map[string][]string{}
	for rows.Next() {
		var role, key string
		if err := rows.Scan(&role, &key); err != nil {
			return nil, err
		}
		roles[role] = append(roles[role], key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &Matrix{Version: version, Permissions: perms, Roles: roles}, nil
}

// PermissionsForRoles returns the deduplicated union of permission keys granted
// to any of the given roles, plus the current matrix version. This is the login
// resolver: SERVICE/USER roles with no grants simply contribute nothing.
func (s *Store) PermissionsForRoles(ctx context.Context, roles []string) ([]string, int64, error) {
	version, err := s.Version(ctx)
	if err != nil {
		return nil, 0, err
	}
	if len(roles) == 0 {
		return nil, version, nil
	}

	rows, err := s.pool.Query(ctx,
		`SELECT DISTINCT permission_key FROM rbac_role_permissions
		 WHERE role = ANY($1) ORDER BY permission_key`, roles)
	if err != nil {
		return nil, 0, fmt.Errorf("permissions for roles: %w", err)
	}
	defer rows.Close()

	var perms []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, 0, err
		}
		perms = append(perms, k)
	}
	return perms, version, rows.Err()
}

// SetRolePermissions replaces the grant set for one role with exactly perms,
// validating that every key exists in the catalog, and bumps the matrix
// version. Runs in a transaction so the matrix is never left half-updated.
// Returns the new version.
func (s *Store) SetRolePermissions(ctx context.Context, role string, perms []string) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Validate keys against the catalog (reject unknown permissions).
	for _, k := range perms {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rbac_permissions WHERE key=$1)`, k).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			return 0, fmt.Errorf("unknown permission key: %q", k)
		}
	}

	if _, err := tx.Exec(ctx, `DELETE FROM rbac_role_permissions WHERE role=$1`, role); err != nil {
		return 0, err
	}
	for _, k := range perms {
		if _, err := tx.Exec(ctx,
			`INSERT INTO rbac_role_permissions (role, permission_key) VALUES ($1,$2)`, role, k); err != nil {
			return 0, err
		}
	}

	var newVersion int64
	if err := tx.QueryRow(ctx,
		`UPDATE rbac_meta SET version = version + 1, updated_at = NOW() WHERE singleton RETURNING version`).
		Scan(&newVersion); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return newVersion, nil
}
