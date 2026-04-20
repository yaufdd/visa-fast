package db

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Hotel struct {
	ID        string    `json:"id"`
	NameEn    string    `json:"name_en"`
	NameRu    *string   `json:"name_ru"`
	City      *string   `json:"city"`
	Address   *string   `json:"address"`
	Phone     *string   `json:"phone"`
	IsGlobal  bool      `json:"is_global"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ListHotels returns global hotels (org_id IS NULL) plus the calling
// org's private hotels. Private ones come first.
func ListHotels(ctx context.Context, pool *pgxpool.Pool, orgID string) ([]Hotel, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, name_en, name_ru, city, address, phone,
		        (org_id IS NULL) AS is_global,
		        created_at, updated_at
		   FROM hotels
		  WHERE org_id IS NULL OR org_id = $1
		  ORDER BY (org_id IS NOT NULL) DESC, name_en`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hotel{}
	for rows.Next() {
		var h Hotel
		if err := rows.Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone,
			&h.IsGlobal, &h.CreatedAt, &h.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

// GetHotel returns a hotel visible to the org (global or private).
func GetHotel(ctx context.Context, pool *pgxpool.Pool, orgID, id string) (*Hotel, error) {
	var h Hotel
	err := pool.QueryRow(ctx,
		`SELECT id, name_en, name_ru, city, address, phone,
		        (org_id IS NULL) AS is_global,
		        created_at, updated_at
		   FROM hotels
		  WHERE id = $1 AND (org_id IS NULL OR org_id = $2)`, id, orgID,
	).Scan(&h.ID, &h.NameEn, &h.NameRu, &h.City, &h.Address, &h.Phone,
		&h.IsGlobal, &h.CreatedAt, &h.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// CreateHotel always creates as private to the calling org.
func CreateHotel(ctx context.Context, pool *pgxpool.Pool, orgID string, h Hotel) (string, error) {
	var id string
	err := pool.QueryRow(ctx,
		`INSERT INTO hotels (org_id, name_en, name_ru, city, address, phone)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		orgID, h.NameEn, h.NameRu, h.City, h.Address, h.Phone,
	).Scan(&id)
	return id, err
}

// UpdateHotel can only update PRIVATE hotels (org_id = $orgID).
// Global hotels (org_id IS NULL) are read-only for all orgs.
// Returns (false, nil) when not found or is global.
func UpdateHotel(ctx context.Context, pool *pgxpool.Pool, orgID, id string, h Hotel) (bool, error) {
	tag, err := pool.Exec(ctx,
		`UPDATE hotels SET name_en = $1, name_ru = $2, city = $3,
		                   address = $4, phone = $5, updated_at = NOW()
		  WHERE id = $6 AND org_id = $7`,
		h.NameEn, h.NameRu, h.City, h.Address, h.Phone, id, orgID)
	return tag.RowsAffected() > 0, err
}
