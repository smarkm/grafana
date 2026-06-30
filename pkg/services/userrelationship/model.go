package userrelationship

import "errors"

var ErrNotFound = errors.New("user relationship not found")

// UserRelationship represents the relationship between a SuperId and associated CustomerIds.
type UserRelationship struct {
	SuperId     string `json:"superId" xorm:"super_id pk"`
	CustomerIds string `json:"customerIds" xorm:"customer_ids"`
}

func (UserRelationship) TableName() string {
	return "user_relationship"
}
