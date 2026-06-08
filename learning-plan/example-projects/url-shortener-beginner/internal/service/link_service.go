package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net/url"

	"urlshortener/internal/domain"
)

const (
	codeLength   = 6
	maxCodeTries = 5
	codeAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
)

// ErrInvalidURL is returned when the supplied URL is not an absolute http(s) URL.
var ErrInvalidURL = errors.New("invalid url: must be an absolute http or https url")

// LinkService holds the business logic for shortening and resolving links.
// It depends on the repository interface, never on a concrete database type.
type LinkService struct {
	repo domain.LinkRepository
}

// NewLinkService injects the repository the service will use.
func NewLinkService(repo domain.LinkRepository) *LinkService {
	return &LinkService{repo: repo}
}

// Shorten validates the URL, generates a unique short code, and stores it.
func (s *LinkService) Shorten(ctx context.Context, longURL string) (*domain.Link, error) {
	if !validURL(longURL) {
		return nil, ErrInvalidURL
	}
	for try := 0; try < maxCodeTries; try++ {
		code, err := generateCode()
		if err != nil {
			return nil, fmt.Errorf("generate code: %w", err)
		}
		link := &domain.Link{Code: code, LongURL: longURL}
		err = s.repo.Create(ctx, link)
		if err == nil {
			return link, nil
		}
		if errors.Is(err, domain.ErrDuplicateCode) {
			continue // collision — try a fresh code
		}
		return nil, err
	}
	return nil, errors.New("could not generate a unique code, please retry")
}

// Resolve returns the link for a code and records the click.
func (s *LinkService) Resolve(ctx context.Context, code string) (*domain.Link, error) {
	link, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if err := s.repo.IncrementClicks(ctx, code); err != nil {
		return nil, err
	}
	link.Clicks++ // reflect the click we just recorded
	return link, nil
}

// Stats returns the link with its current click count (no increment).
func (s *LinkService) Stats(ctx context.Context, code string) (*domain.Link, error) {
	return s.repo.GetByCode(ctx, code)
}

// validURL accepts only absolute http/https URLs with a host.
func validURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.IsAbs() && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// generateCode returns a cryptographically-random base62 code.
func generateCode() (string, error) {
	buf := make([]byte, codeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i, b := range buf {
		buf[i] = codeAlphabet[int(b)%len(codeAlphabet)]
	}
	return string(buf), nil
}
