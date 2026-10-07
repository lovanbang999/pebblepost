package secrets

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// MigrationPlan describes secrets that will be moved during a migration.
type MigrationPlan struct {
	EnvName string
	Keys    []string
	SrcType SecretBackendType
	DstType SecretBackendType
}

// MigrateOptions controls migration behaviour.
type MigrateOptions struct {
	// Confirm is called with the plan before any changes are made.
	// Return false to abort without making any changes.
	Confirm func(plan MigrationPlan) bool
	// Warn is called for non-fatal issues (e.g. key already exists in dst).
	Warn func(msg string)
}

// Migrate moves all secrets for envName from src to dst.
// Steps:
//  1. List keys in src.
//  2. Call opts.Confirm; abort if false.
//  3. Back up the source secret file (file backend only).
//  4. For each key: Get from src → Set in dst.
//  5. Delete each key from src.
//  6. On any error in step 4/5: restore backup and return error.
//
// Returns the backup path (empty string if src is not a file store).
func Migrate(envName string, src, dst SecretStore, opts MigrateOptions) (backupPath string, err error) {
	if src.Backend() == dst.Backend() {
		return "", fmt.Errorf("source and destination backends are the same (%s)", src.Backend())
	}

	keys, err := src.List(envName)
	if err != nil {
		return "", fmt.Errorf("list secrets from %s: %w", src.Backend(), err)
	}
	if len(keys) == 0 {
		return "", nil
	}

	plan := MigrationPlan{
		EnvName: envName,
		Keys:    keys,
		SrcType: src.Backend(),
		DstType: dst.Backend(),
	}

	if opts.Confirm != nil && !opts.Confirm(plan) {
		return "", nil // user declined
	}

	// Backup source file if it is a FileStore.
	if fs, ok := src.(*FileStore); ok {
		backupPath, err = backupFile(fs.Path(envName))
		if err != nil {
			return "", fmt.Errorf("backup secret file: %w", err)
		}
	}

	// Transfer each key; roll back on failure.
	transferred := make([]string, 0, len(keys))
	for _, key := range keys {
		val, err := src.Get(envName, key)
		if err != nil {
			rollbackMigration(envName, transferred, dst, backupPath, src)
			return "", fmt.Errorf("read %s/%s from %s: %w", envName, key, src.Backend(), err)
		}
		if err := dst.Set(envName, key, val); err != nil {
			rollbackMigration(envName, transferred, dst, backupPath, src)
			return "", fmt.Errorf("write %s/%s to %s: %w", envName, key, dst.Backend(), err)
		}
		transferred = append(transferred, key)
	}

	// Remove originals from src (best-effort; backup exists for recovery).
	for _, key := range keys {
		if err := src.Delete(envName, key); err != nil {
			if opts.Warn != nil {
				opts.Warn(fmt.Sprintf("could not remove %s/%s from %s after migration: %v", envName, key, src.Backend(), err))
			}
		}
	}

	return backupPath, nil
}

func backupFile(srcPath string) (string, error) {
	data, err := os.ReadFile(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	bak := fmt.Sprintf("%s.bak.%d", srcPath, time.Now().Unix())
	if err := os.WriteFile(bak, data, 0600); err != nil {
		return "", err
	}
	return bak, nil
}

// rollbackMigration undoes a partial migration by deleting already-transferred
// keys from dst and restoring the backup file in src (if applicable).
func rollbackMigration(envName string, transferred []string, dst SecretStore, backupPath string, src SecretStore) {
	for _, key := range transferred {
		_ = dst.Delete(envName, key)
	}
	if backupPath != "" {
		if fs, ok := src.(*FileStore); ok {
			data, err := os.ReadFile(backupPath)
			if err == nil {
				_ = os.WriteFile(fs.Path(envName), data, 0600)
			}
		}
	}
}

// Rollback restores a secret file from its backup and removes any transferred keys
// from the destination store.
func Rollback(envName, backupPath string, dst SecretStore) error {
	if backupPath == "" {
		return fmt.Errorf("no backup path provided")
	}
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return fmt.Errorf("read backup: %w", err)
	}

	// Calculate target file path by stripping .bak.<timestamp>
	destPath := backupPath
	if idx := strings.Index(destPath, ".bak."); idx != -1 {
		destPath = destPath[:idx]
	}

	if err := os.WriteFile(destPath, data, 0600); err != nil {
		return fmt.Errorf("restore file: %w", err)
	}

	// If destination store exists, remove any migrated keys
	if dst != nil {
		if keys, err := dst.List(envName); err == nil {
			for _, k := range keys {
				_ = dst.Delete(envName, k)
			}
		}
	}
	return nil
}
