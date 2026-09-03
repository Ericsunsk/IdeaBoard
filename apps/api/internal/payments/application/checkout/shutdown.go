package checkout

import paymentconfiguration "ideaboard/api/internal/payments/application/configuration"

// PaymentCheckoutShutdown is owned by configuration publication while
// checkout consumes it to expire stale sessions.
type PaymentCheckoutShutdown = paymentconfiguration.PaymentCheckoutShutdown
