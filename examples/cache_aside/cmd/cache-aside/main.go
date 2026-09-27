package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	cacheaside "spec-guardian/examples/cache-aside"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/valkey-io/valkey-go"
)

const (
	databaseURL = "postgres://postgres@localhost:15432/postgres?sslmode=disable"
	valkeyAddr  = "localhost:16379"
	demoValue   = "written"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	database, err := openDatabase(ctx)
	if err != nil {
		return err
	}
	defer database.Close()

	cache, err := openCache()
	if err != nil {
		return err
	}
	defer cache.Close()

	key := fmt.Sprintf("cache-aside-demo:%d", time.Now().UnixNano())
	application := cacheaside.NewApplication(database, cache)
	return demonstrateCacheAside(ctx, application, database, cache, key)
}

func openDatabase(ctx context.Context) (*sql.DB, error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := database.PingContext(ctx); err != nil {
		database.Close()
		return nil, fmt.Errorf("connect to database: %w", err)
	}
	return database, nil
}

func openCache() (valkey.Client, error) {
	cache, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	if err != nil {
		return nil, fmt.Errorf("connect to cache: %w", err)
	}
	return cache, nil
}

func demonstrateCacheAside(ctx context.Context, application *cacheaside.Application, database *sql.DB, cache valkey.Client, key string) error {
	fmt.Printf("Cache-aside demo for key %q\n", key)

	if err := readWithoutValues(ctx, application, key); err != nil {
		return err
	}
	if err := write(ctx, application, database, cache, key); err != nil {
		return err
	}
	if err := readAfterWrite(ctx, application, cache, key); err != nil {
		return err
	}

	fmt.Println("Cache-aside validation completed")
	return nil
}

func readWithoutValues(ctx context.Context, application *cacheaside.Application, key string) error {
	output, err := application.Read(ctx, &cacheaside.ReadInput{Key: key})
	if err != nil {
		return fmt.Errorf("Application.Read on missing key: %w", err)
	}
	if output == nil || output.Key != key || output.Value != nil {
		return fmt.Errorf("Application.Read on missing key: got %+v, want no value for %q", output, key)
	}

	fmt.Println("1. Application.Read: no value for the key")
	return nil
}

func write(ctx context.Context, application *cacheaside.Application, database *sql.DB, cache valkey.Client, key string) error {
	output, err := application.Write(ctx, &cacheaside.WriteInput{Key: key, Value: demoValue})
	if err != nil {
		return fmt.Errorf("Application.Write: %w", err)
	}
	if output == nil || output.Key != key || output.Value != demoValue {
		return fmt.Errorf("Application.Write: got %+v, want %q for %q", output, demoValue, key)
	}

	if err := verifyDatabaseValue(ctx, database, key, demoValue); err != nil {
		return err
	}
	if err := verifyCacheMiss(ctx, cache, key); err != nil {
		return err
	}

	fmt.Printf("2. Application.Write(%q): DB contains %q; cache entry is absent\n", demoValue, demoValue)
	return nil
}

func readAfterWrite(ctx context.Context, application *cacheaside.Application, cache valkey.Client, key string) error {
	output, err := application.Read(ctx, &cacheaside.ReadInput{Key: key})
	if err != nil {
		return fmt.Errorf("Application.Read after write: %w", err)
	}
	if output == nil || output.Key != key || output.Value == nil || *output.Value != demoValue {
		return fmt.Errorf("Application.Read after write: got %+v, want %q for %q", output, demoValue, key)
	}

	if err := verifyCacheValue(ctx, cache, key, demoValue); err != nil {
		return err
	}

	fmt.Printf("3. Application.Read: returned %q and populated the cache with %q\n", *output.Value, demoValue)
	return nil
}

func verifyDatabaseValue(ctx context.Context, database *sql.DB, key, want string) error {
	var got string
	if err := database.QueryRowContext(ctx, "SELECT value FROM records WHERE KEY = $1", key).Scan(&got); err != nil {
		return fmt.Errorf("read database after Application.Write: %w", err)
	}
	if got != want {
		return fmt.Errorf("database value after Application.Write: got %q, want %q", got, want)
	}
	return nil
}

func verifyCacheMiss(ctx context.Context, cache valkey.Client, key string) error {
	_, found, err := readCacheValue(ctx, cache, key)
	if err != nil {
		return err
	}
	if found {
		return errors.New("cache entry exists after Application.Write")
	}
	return nil
}

func verifyCacheValue(ctx context.Context, cache valkey.Client, key, want string) error {
	got, found, err := readCacheValue(ctx, cache, key)
	if err != nil {
		return err
	}
	if !found || got != want {
		return fmt.Errorf("cache value after Application.Read: got %q (found=%t), want %q", got, found, want)
	}
	return nil
}

func readCacheValue(ctx context.Context, cache valkey.Client, key string) (string, bool, error) {
	value, err := cache.Do(ctx, cache.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read cache: %w", err)
	}
	return value, true, nil
}
