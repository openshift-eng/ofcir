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

type AcquisitionError struct {
	id  string
	err error
}

func (a AcquisitionError) Error() string {
	return fmt.Sprintf("Error acquiring resource id %s: %s", a.id, a.err)
}

func (a AcquisitionError) Unrwap() error {
	return a.err
}

func NewAcquisitionError(id string, err error) error {
	return AcquisitionError{
		id:  id,
		err: err,
	}
}
