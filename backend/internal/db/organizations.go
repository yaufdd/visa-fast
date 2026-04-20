// Package db contains tenant-aware repository functions — every
// owned-entity function takes orgID as a mandatory parameter.
package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Organization struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CreateOrganization inserts a new org and returns the new ID.
// Caller must handle unique-violation on `slug` by retrying with a new slug.
func CreateOrganization(ctx context.Context, pool *pgxpool.Pool, name, slug string) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`,
		name, slug,
	).Scan(&id)
	return id, err
}

// GetOrganizationBySlug is used by public slug-based form endpoints.
func GetOrganizationBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (*Organization, error) {
	var o Organization
	err := pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at
		   FROM organizations WHERE slug = $1`,
		slug,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// GetOrganizationByID is used by the /me endpoint.
func GetOrganizationByID(ctx context.Context, pool *pgxpool.Pool, id string) (*Organization, error) {
	var o Organization
	err := pool.QueryRow(ctx,
		`SELECT id, name, slug, created_at, updated_at
		   FROM organizations WHERE id = $1`,
		id,
	).Scan(&o.ID, &o.Name, &o.Slug, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}
