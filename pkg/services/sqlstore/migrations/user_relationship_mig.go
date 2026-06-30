package migrations

import (
	. "github.com/grafana/grafana/pkg/services/sqlstore/migrator"
)

func addUserRelationshipTableMigration(mg *Migrator) {
	userRelationshipV1 := Table{
		Name: "user_relationship",
		Columns: []*Column{
			{Name: "super_id", Type: DB_Text, Nullable: false},
			{Name: "customer_ids", Type: DB_Text, Nullable: false},
		},
		Indices: []*Index{
			{Cols: []string{"super_id"}, Type: UniqueIndex},
		},
	}

	mg.AddMigration("create user_relationships table", NewAddTableMigration(userRelationshipV1))
	mg.AddMigration("add unique index user_relationships.super_id", NewAddIndexMigration(userRelationshipV1, userRelationshipV1.Indices[0]))
}
