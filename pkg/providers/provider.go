package providers

// Resource represents a specific instance reserved and/or created
// by the provider for a given request
type Resource struct {
	// A unique identifier used to reference the resource
	Id string
	// The public IPv4 address of the resource
	Address string
	// Extra information specific to the provider
	Metadata string
}

type Provider interface {
	// Request a new resource. Resource allocation may take some time,
	// so it is expected that the provider will reply immediately
	// with a Resource containing at least the Id
	Acquire(poolSize int, poolName string, poolType string) (Resource, error)

	// Check if the acquire operation completed or not. It could be
	// used to poll a resource for its public address after an Acqure
	AcquireCompleted(id string) (bool, Resource, error)

	// Remove all data from the resource, preparing it for a new
	// request
	Clean(id string) error

	// Check if the clean operation completed or not.
	CleanCompleted(id string) (bool, error)

	// Release the specified resource, to be used for a new request
	Release(id string) error
}
