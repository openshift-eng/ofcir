package providers

import "fmt"

type ResourceNotFoundError struct {
	id string
}

func NewResourceNotFoundError(id string) error {
	return ResourceNotFoundError{
		id: id,
	}
}

func (r ResourceNotFoundError) Error() string {
	return fmt.Sprintf("Resource %s not found", r.id)
}
