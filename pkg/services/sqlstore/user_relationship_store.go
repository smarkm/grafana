package sqlstore

import (
	"fmt"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/models"
)

// Initialize the handlers
func init() {
	bus.AddHandler("sql", SaveUserRelationship)
	bus.AddHandler("sql", UpdateUserRelationship)
	bus.AddHandler("sql", DeleteUserRelationship)
	bus.AddHandler("sql", QueryAllUserRelationships)
	bus.AddHandler("sql", QueryUserRelationshipBySuperID)
}

// SaveUserRelationship handles the SaveUserRelationshipCommand
func SaveUserRelationship(cmd *models.SaveUserRelationshipCommand) error {
	_, err := x.Insert(cmd.Data)
	if err != nil {
		return fmt.Errorf("failed to save user relationship: %w", err)
	}

	return nil
}

// UpdateUserRelationship handles the UpdateUserRelationshipCommand
func UpdateUserRelationship(cmd *models.UpdateUserRelationshipCommand) error {
	affected, err := x.ID(cmd.Data.SuperId).Update(cmd.Data)
	if err != nil {
		return fmt.Errorf("failed to update user relationship: %w", err)
	}

	cmd.RowsAffected = affected
	return nil
}

// DeleteUserRelationship handles the DeleteUserRelationshipCommand
func DeleteUserRelationship(cmd *models.DeleteUserRelationshipCommand) error {
	affected, err := x.ID(cmd.SuperId).Delete(&models.UserRelationship{})
	if err != nil {
		return fmt.Errorf("failed to delete user relationship: %w", err)
	}

	cmd.RowsAffected = affected
	return nil
}

// QueryAllUserRelationships handles the QueryAllUserRelationshipsQuery
func QueryAllUserRelationships(query *models.QueryAllUserRelationshipsQuery) error {
	var relationships []models.UserRelationship
	err := x.Find(&relationships)
	if err != nil {
		return fmt.Errorf("failed to query all user relationships: %w", err)
	}

	query.Result = relationships
	return nil
}

// QueryUserRelationshipBySuperID handles the QueryUserRelationshipBySuperIDQuery
func QueryUserRelationshipBySuperID(query *models.QueryUserRelationshipBySuperIdQuery) error {
	var relationship models.UserRelationship
	has, err := x.Where("super_id = ?", query.SuperId).Get(&relationship)
	if err != nil {
		return fmt.Errorf("failed to query user relationship by superID: %w", err)
	}
	if !has {
		return fmt.Errorf("user relationship with superID '%s' not found", query.SuperId)
	}

	query.Result = &relationship
	return nil
}
