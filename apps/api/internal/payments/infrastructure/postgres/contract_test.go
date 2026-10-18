package postgres_test

import (
	paymentpersistence "ideaboard/api/internal/payments/application/persistence"
	postgres "ideaboard/api/internal/payments/infrastructure/postgres"
)

var (
	_ paymentpersistence.StripeConfigurationStore     = (*postgres.Repository)(nil)
	_ paymentpersistence.StripeWebhookInboxStore      = (*postgres.Repository)(nil)
	_ paymentpersistence.PaymentOrderConvergenceStore = (*postgres.Repository)(nil)
)
