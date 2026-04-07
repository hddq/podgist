package main

import (
	"context"
	"fmt"
	"log"

	"github.com/hddq/podgist/internal/service"
	"github.com/hddq/podgist/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := "postgres://podgist:podgist@127.0.0.1:5432/podgist?sslmode=disable"
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("Error connecting: %v", err)
	}
	defer pool.Close()

	s := store.New(pool)
	auth := service.NewAuthService(s, 10)

	user, err := auth.CreateUser(ctx, "admin", "test")
	if err != nil {
		log.Fatalf("Error creating user: %v", err)
	}

	fmt.Printf("User created: %s (ID: %d)\n", user.Username, user.ID)
}