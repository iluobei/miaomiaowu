package substore

import (
	"fmt"
	"sync"
)

// ProducerFactory creates and manages producers
type ProducerFactory struct {
	producers map[string]Producer
	mu        sync.RWMutex
}

// NewProducerFactory creates a new ProducerFactory
func NewProducerFactory() *ProducerFactory {
	factory := &ProducerFactory{
		producers: make(map[string]Producer),
	}

	// Register default producers
	factory.Register(NewClashProducer())
	factory.Register(NewClashMetaProducer())
	factory.Register(NewSurfboardProducer())
	factory.Register(NewURIProducer())
	factory.Register(NewV2RayProducer())
	factory.Register(NewShadowrocketProducer())
	factory.Register(NewSurgeProducer())
	factory.Register(NewSurgeMacProducer())
	factory.Register(NewStashProducer())
	factory.Register(NewQXProducer())
	factory.Register(NewLoonProducer())
	factory.Register(NewSingboxProducer())
	factory.Register(NewEgernProducer())

	return factory
}

// Register registers a producer
func (f *ProducerFactory) Register(producer Producer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.producers[producer.GetType()] = producer
}

// GetProducer returns a producer by type
func (f *ProducerFactory) GetProducer(producerType string) (Producer, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	producer, ok := f.producers[producerType]
	if !ok {
		return nil, fmt.Errorf("producer type '%s' not found", producerType)
	}
	return producer, nil
}

// GetSupportedTypes returns all supported producer types
func (f *ProducerFactory) GetSupportedTypes() []string {
	f.mu.RLock()
	defer f.mu.RUnlock()

	types := make([]string, 0, len(f.producers))
	for t := range f.producers {
		types = append(types, t)
	}
	return types
}

// ConvertProxies converts proxies to the specified format
func (f *ProducerFactory) ConvertProxies(proxies []Proxy, targetFormat string, opts *ProduceOptions) (interface{}, error) {
	producer, err := f.GetProducer(targetFormat)
	if err != nil {
		return nil, err
	}

	return producer.Produce(proxies, "", opts)
}

// Default global factory instance
var defaultFactory *ProducerFactory
var once sync.Once

// GetDefaultFactory returns the default global factory
func GetDefaultFactory() *ProducerFactory {
	once.Do(func() {
		defaultFactory = NewProducerFactory()
	})
	return defaultFactory
}

// ConvertToClash is a convenience function to convert proxies to Clash format
func ConvertToClash(proxies []Proxy, includeUnsupported bool) (string, error) {
	opts := &ProduceOptions{
		IncludeUnsupportedProxy: includeUnsupported,
	}

	result, err := GetDefaultFactory().ConvertProxies(proxies, "clash", opts)
	if err != nil {
		return "", err
	}

	if str, ok := result.(string); ok {
		return str, nil
	}

	return "", fmt.Errorf("unexpected result type")
}
