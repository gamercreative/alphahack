package services

import "context"

// STUB: the auth slice builder replaces this file.
func (s *EnquiryService) Authenticate(ctx context.Context, authorization string) (Actor, error) {
	return Actor{}, Unauthorized("not implemented")
}
