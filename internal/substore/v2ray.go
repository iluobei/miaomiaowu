package substore

import (
	"encoding/base64"
	"strings"
)

// V2RayProducer implements V2Ray subscription format (base64 encoded URIs)
type V2RayProducer struct {
	producerType string
	uriProducer  *URIProducer
}

// NewV2RayProducer creates a new V2Ray producer
func NewV2RayProducer() *V2RayProducer {
	return &V2RayProducer{
		producerType: "v2ray",
		uriProducer:  NewURIProducer(),
	}
}

// GetType returns the producer type
func (p *V2RayProducer) GetType() string {
	return p.producerType
}

// Produce converts proxies to V2Ray subscription format (base64 encoded URIs)
func (p *V2RayProducer) Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error) {
	var uris []string

	for _, proxy := range proxies {
		// Try to encode each proxy as URI
		uri, err := p.uriProducer.ProduceOne(proxy)
		if err != nil {
			// Skip proxies that cannot be encoded
			// In production, you might want to log this
			continue
		}
		uris = append(uris, uri)
	}

	// Join all URIs with newline and encode as base64
	content := strings.Join(uris, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(content))

	return encoded, nil
}
