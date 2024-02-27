package k8s

import (
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeLazyClientInitializer struct {
	lazyClientInitializerInterface
	fakeClientBuilder *fake.ClientBuilder
}

func (b *fakeLazyClientInitializer) initialize(s *LazyClient) {
	s.client = b.fakeClientBuilder.Build()
}

func NewFakeClient(clientBuilder *fake.ClientBuilder) *LazyClient {
	return &LazyClient{&fakeLazyClientInitializer{fakeClientBuilder: clientBuilder}, nil, nil, "", nil}
}
