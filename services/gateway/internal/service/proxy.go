package service

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

type ProxyService struct {
	serviceURLs config.ServiceURLs
	client      *http.Client
	log         *logger.Logger
}

func NewProxyService(serviceURLs config.ServiceURLs, log *logger.Logger) *ProxyService {
	return &ProxyService{
		serviceURLs: serviceURLs,
		client:      &http.Client{},
		log:         log,
	}
}

// Forward forwards the request to the appropriate service
func (s *ProxyService) Forward(r *http.Request, service string) (*http.Response, error) {
	targetURL, err := s.getServiceURL(service)
	if err != nil {
		return nil, err
	}

	// Build target URL with original path and query
	fullURL := targetURL + r.URL.Path
	if r.URL.RawQuery != "" {
		fullURL += "?" + r.URL.RawQuery
	}

	// Create new request
	req, err := http.NewRequestWithContext(r.Context(), r.Method, fullURL, r.Body)
	if err != nil {
		return nil, errors.Internal(fmt.Sprintf("create proxy request: %v", err))
	}

	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Forward request
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, errors.Internal(fmt.Sprintf("forward request: %v", err))
	}

	return resp, nil
}

func (s *ProxyService) getServiceURL(service string) (string, error) {
	switch strings.ToLower(service) {
	case "auth":
		return s.serviceURLs.AuthService, nil
	case "catalog":
		return s.serviceURLs.CatalogService, nil
	case "player":
		return s.serviceURLs.PlayerService, nil
	case "rooms":
		return s.serviceURLs.RoomsService, nil
	case "streaming":
		return s.serviceURLs.StreamingService, nil
	default:
		return "", errors.NotFound("service")
	}
}
