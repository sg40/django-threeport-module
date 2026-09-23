// hand-written migration - see 000001_init.go for the pattern this follows.
package migrations

import (
	"context"
	"database/sql"
	v0 "django-threeport-module/pkg/api/v0"
	"fmt"
	goose "github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationNoTxContext(Up000002, Down000002)
}

// Up000002 adds the EnvVars column to django_definitions. A database that
// already applied 000001 before this field existed never reruns it, so the
// column has to be added explicitly here. AutoMigrate only creates missing
// columns and indexes - it does not touch existing data - so this is safe to
// run against a table that already has rows.
func Up000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.AutoMigrate(&v0.DjangoDefinition{}); err != nil {
		return fmt.Errorf("could not run gorm AutoMigrate: %w", err)
	}

	return nil
}

func Down000002(ctx context.Context, db *sql.DB) error {
	gormDb, err := getGormDbFromContext(ctx)
	if err != nil {
		return err
	}

	if err := gormDb.Migrator().DropColumn(&v0.DjangoDefinition{}, "EnvVars"); err != nil {
		return fmt.Errorf("could not drop env_vars column: %w", err)
	}

	return nil
}
