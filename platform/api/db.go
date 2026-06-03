// AND-105 Task 4: Go API Migration — CSV to PostgreSQL

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var db *pgxpool.Pool

func InitDB() error {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	if port == "" {
		port = "5432"
	}

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s", host, port, user, password, dbname)

	var (
		pool *pgxpool.Pool
		err  error
	)
	for attempt := 1; attempt <= 10; attempt++ {
		pool, err = pgxpool.New(context.Background(), dsn)
		if err != nil {
			log.Printf("db connect attempt %d/10: %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		if err = pool.Ping(context.Background()); err != nil {
			pool.Close()
			log.Printf("db ping attempt %d/10: %v", attempt, err)
			time.Sleep(2 * time.Second)
			continue
		}
		db = pool
		return nil
	}
	return fmt.Errorf("database unavailable after 10 attempts: %w", err)
}
