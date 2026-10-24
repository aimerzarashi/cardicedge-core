package graphql

type Resolver struct {
	version string
}

func NewResolver(version string) *Resolver {
	return &Resolver{version: version}
}
