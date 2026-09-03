package submission

import (
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	taskqueue "ideaboard/api/internal/tasking/infrastructure/queue"

	"github.com/jackc/pgx/v5/pgxpool"
)

func newTestStore(
	pool *pgxpool.Pool,
	repository SubmissionTaskStore,
	client taskqueue.RiverTxClient,
	queueName string,
	maxRetry int,
	billingService *taskbilling.Service,
) *Store {
	return NewStore(pool, repository, taskqueue.NewTransactionalEnqueuer(client, queueName, maxRetry), billingService)
}
