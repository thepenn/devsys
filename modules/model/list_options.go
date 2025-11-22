package model

// ListOptions captures pagination parameters for list APIs.
type ListOptions struct {
	All     bool
	Page    int
	PerPage int
}
