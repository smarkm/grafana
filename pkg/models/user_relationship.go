package models

// UserRelationship represents the relationship between a SuperId and associated CustomerIds
type UserRelationship struct {
	SuperId     string `json:"superId"`
	CustomerIds string `json:"customerIds"`
}

// Command for saving a UserRelationship
type SaveUserRelationshipCommand struct {
	Data   UserRelationship `json:"data"`
	Result int64            `json:"-"` // The result of the insert operation
}

// Command for updating a UserRelationship
type UpdateUserRelationshipCommand struct {
	Data         UserRelationship `json:"data"`
	RowsAffected int64            `json:"-"` // The number of rows updated
}

// Command for deleting a UserRelationship
type DeleteUserRelationshipCommand struct {
	SuperId      string `json:"superId"`
	RowsAffected int64  `json:"-"` // The number of rows deleted
}

// Query for all UserRelationships
type QueryAllUserRelationshipsQuery struct {
	Result []UserRelationship `json:"result"`
}

// Query for a UserRelationship by SuperId
type QueryUserRelationshipBySuperIdQuery struct {
	SuperId string            `json:"superId"`
	Result  *UserRelationship `json:"result"`
}
