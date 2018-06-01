package entity

import (
	"errors"
)

var ErrNotFound = errors.New("Not Found")

var ErrCannotBeDeleted = errors.New("Cannot Be Deleted")
