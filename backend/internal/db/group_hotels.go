package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type GroupHotel struct {
	ID         string    `json:"id"`
	GroupID    string    `json:"group_id"`
	SubgroupID *string   `json:"subgroup_id"`
	HotelID    string    `json:"hotel_id"`
	CheckIn    time.Time `json:"check_in"`
	CheckOut   time.Time `json:"check_out"`
	RoomType   *string   `json:"room_type"`
	SortOrder  int       `json:"sort_order"`
}

func ListGroupHotels(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string) ([]GroupHotel, error) {
	rows, err := pool.Query(ctx,
		`SELECT id, group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order
		   FROM group_hotels WHERE group_id = $1 AND org_id = $2
		   ORDER BY sort_order`, groupID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []GroupHotel{}
	for rows.Next() {
		var gh GroupHotel
		if err := rows.Scan(&gh.ID, &gh.GroupID, &gh.SubgroupID, &gh.HotelID,
			&gh.CheckIn, &gh.CheckOut, &gh.RoomType, &gh.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, gh)
	}
	return out, nil
}

func UpsertGroupHotels(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string, hotels []GroupHotel) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx,
		`DELETE FROM group_hotels WHERE group_id = $1 AND org_id = $2`, groupID, orgID); err != nil {
		return err
	}
	for i, gh := range hotels {
		_, err = tx.Exec(ctx,
			`INSERT INTO group_hotels (org_id, group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			orgID, groupID, gh.SubgroupID, gh.HotelID, gh.CheckIn, gh.CheckOut, gh.RoomType, i)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// AppendGroupHotel — used by the voucher-parser auto-insert flow.
// Computes sort_order as MAX+1 for the group.
func AppendGroupHotel(ctx context.Context, pool *pgxpool.Pool, orgID, groupID string, gh GroupHotel) error {
	_, err := pool.Exec(ctx,
		`INSERT INTO group_hotels (org_id, group_id, subgroup_id, hotel_id, check_in, check_out, room_type, sort_order)
		 SELECT $1, $2, $3, $4, $5, $6, $7,
		        COALESCE((SELECT MAX(sort_order) + 1 FROM group_hotels WHERE group_id = $2 AND org_id = $1), 0)`,
		orgID, groupID, gh.SubgroupID, gh.HotelID, gh.CheckIn, gh.CheckOut, gh.RoomType)
	return err
}
