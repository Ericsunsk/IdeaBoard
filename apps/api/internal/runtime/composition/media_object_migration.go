package composition

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"ideaboard/api/internal/media/storage/infrastructure/local"
	"ideaboard/api/internal/platform/config"
	"ideaboard/api/internal/platform/database"
)

type mediaObjectMigration struct {
	source   string
	target   string
	required bool
}

func prepareMediaObjectMigration(ctx context.Context, cfg *config.Config, databaseConfig config.DatabaseConfig) error {
	db, err := database.NewPostgres(ctx, databaseConfig)
	if err != nil {
		return err
	}
	defer db.Close()

	var migrations []mediaObjectMigration
	for _, table := range []string{"upload_sessions", "media_assets", "users"} {
		var exists bool
		if err := db.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			continue
		}
		query := "SELECT storage_key, false FROM upload_sessions WHERE storage_key LIKE 'ws-uploads/%'"
		switch table {
		case "media_assets":
			query = "SELECT storage_key, archived_at IS NULL FROM media_assets WHERE storage_key LIKE 'ws-uploads/%'"
		case "users":
			query = "SELECT avatar, true FROM users WHERE avatar LIKE '/uploads/profile/%'"
		}
		rows, err := db.Query(ctx, query)
		if err != nil {
			return err
		}
		for rows.Next() {
			var source string
			var required bool
			if err := rows.Scan(&source, &required); err != nil {
				rows.Close()
				return err
			}
			source = strings.TrimPrefix(source, "/")
			target := "uploads/ws/" + strings.TrimPrefix(source, "ws-uploads/")
			if table == "users" {
				parts := strings.SplitN(strings.TrimPrefix(source, "uploads/profile/"), "/", 3)
				if len(parts) != 3 || parts[1] != "avatar" {
					rows.Close()
					return errors.New("invalid stored avatar object key")
				}
				target = "uploads/avatars/" + parts[0] + "/" + parts[2]
			}
			migrations = append(migrations, mediaObjectMigration{source: source, target: target, required: required})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if len(migrations) == 0 {
		return nil
	}
	root, err := os.OpenRoot(local.ResolveLocalStorageRoot(cfg.Storage.LocalRoot))
	if err != nil {
		return err
	}
	defer root.Close()
	for _, migration := range migrations {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := copyMediaObject(root, migration); err != nil {
			return fmt.Errorf("prepare media object keyspace: %w", err)
		}
	}
	return nil
}

func copyMediaObject(root *os.Root, migration mediaObjectMigration) error {
	for _, key := range []string{migration.source, migration.target} {
		if !fs.ValidPath(key) || path.Clean(key) != key {
			return errors.New("invalid media object migration path")
		}
	}
	source, err := root.Open(migration.source)
	if errors.Is(err, os.ErrNotExist) && strings.HasPrefix(migration.source, "ws-uploads/") {
		source, err = root.Open("uploads/" + migration.source)
	}
	if errors.Is(err, os.ErrNotExist) && !migration.required {
		return nil
	}
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("media object source is not a regular file")
	}
	if err := root.MkdirAll(path.Dir(migration.target), 0700); err != nil {
		return err
	}
	temporary := migration.target + ".migration-" + rand.Text()
	target, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temporary) }()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(target, hash), source); err != nil {
		_ = target.Close()
		return err
	}
	if err := target.Close(); err != nil {
		return err
	}
	if err := root.Link(temporary, migration.target); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		existing, openErr := root.Open(migration.target)
		if openErr != nil {
			return openErr
		}
		defer existing.Close()
		existingHash := sha256.New()
		if _, err := io.Copy(existingHash, existing); err != nil {
			return err
		}
		if string(hash.Sum(nil)) != string(existingHash.Sum(nil)) {
			return errors.New("media object migration destination has different content")
		}
	}
	return nil
}
