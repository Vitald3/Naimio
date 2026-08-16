package payments

import (
	"context"
	"errors"
	"testing"
)

type routeRepository struct{ route Route }

func (r routeRepository) GetRoute(context.Context, Domain) (Route, error) { return r.route, nil }

func TestDefaultRegistryRejectsUnsupportedSafeDealRoutes(t *testing.T) {
	r := DefaultRegistry()
	if err := r.ValidateRoute(ProviderYandexPay, true, true, CapabilitySafeDeal, CapabilityPayoutCard); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("Yandex Pay must not be eligible for Safe Deal, got %v", err)
	}
	if err := r.ValidateRoute(ProviderRobokassa, true, true, CapabilitySafeDeal); !errors.Is(err, ErrUnsupportedRoute) {
		t.Fatalf("Robokassa must not be eligible for Safe Deal, got %v", err)
	}
}

func TestDefaultRegistryRequiresEnabledConfiguredProvider(t *testing.T) {
	r := DefaultRegistry()
	if err := r.ValidateRoute(ProviderYooKassa, false, true, CapabilitySafeDeal); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("disabled route accepted: %v", err)
	}
	if err := r.ValidateRoute(ProviderYooKassa, true, false, CapabilitySafeDeal); !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("unconfigured route accepted: %v", err)
	}
	if err := r.ValidateRoute(ProviderYooKassa, true, true, CapabilitySafeDeal, CapabilityPayoutCard); err != nil {
		t.Fatalf("verified YooKassa route rejected: %v", err)
	}
}

func TestProductionRoutingRejectsSandboxProvider(t *testing.T) {
	service := RoutingService{
		Repository:             routeRepository{route: Route{Domain: DomainProSubscription, Provider: ProviderYooKassa, Enabled: true, Configured: true, Environment: EnvironmentSandbox}},
		Registry:               DefaultRegistry(),
		ApplicationEnvironment: EnvironmentProduction,
	}
	_, err := service.Select(context.Background(), DomainProSubscription, CapabilityOneTimePayment)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("sandbox route accepted in production: %v", err)
	}
}
