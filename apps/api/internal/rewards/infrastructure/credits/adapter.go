package creditsadapter

import (
	"context"
	"errors"

	credits "ideaboard/api/internal/credits"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/rewards/application/redemptions"
	rewardsdomain "ideaboard/api/internal/rewards/domain"
	"ideaboard/api/internal/rewards/ports"

	"github.com/jackc/pgx/v5"
)

// Adapter translates the Credits context contract into the narrow Rewards gateway.
type Adapter struct{ service *credits.Service }

func New(service *credits.Service) *Adapter { return &Adapter{service: service} }

func (a *Adapter) ReadWallet(ctx context.Context, userID int64) (redemptions.WalletSnapshot, error) {
	wallet, err := a.service.GetWallet(ctx, userID)
	if errors.Is(err, creditsdomain.ErrWalletNotFound) {
		return redemptions.WalletSnapshot{}, ports.ErrWalletNotFound
	}
	if errors.Is(err, creditsdomain.ErrWalletFrozen) {
		return redemptions.WalletSnapshot{}, rewardsdomain.ErrWalletFrozen
	}
	if err != nil {
		return redemptions.WalletSnapshot{}, err
	}
	return redemptions.WalletSnapshot{Available: wallet.Available}, nil
}

func (a *Adapter) Grant(ctx context.Context, tx pgx.Tx, request redemptions.CreditGrantRequest) (redemptions.CreditGrantResult, error) {
	amount, err := creditsdomain.NewCreditAmount(request.Amount)
	if err != nil {
		return redemptions.CreditGrantResult{}, err
	}
	result, err := a.service.InTx(tx).Grant(ctx, creditsports.GrantCommand{
		UserID: request.UserID,
		Amount: amount,
		Metadata: creditsdomain.CommandMetadata{
			IdempotencyKey:    request.IdempotencyKey,
			Actor:             creditsdomain.Actor{Type: creditsdomain.ActorType(request.ActorType), ID: request.ActorID},
			Source:            request.Source,
			Reason:            request.Reason,
			BusinessReference: creditsdomain.BusinessReference{Type: request.BusinessReferenceType, ID: request.BusinessReferenceID},
		},
	})
	if err != nil {
		if errors.Is(err, creditsdomain.ErrWalletFrozen) {
			return redemptions.CreditGrantResult{}, rewardsdomain.ErrWalletFrozen
		}
		return redemptions.CreditGrantResult{}, err
	}
	return redemptions.CreditGrantResult{TransactionID: result.TransactionID, AvailableBalance: result.Wallet.Available}, nil
}

var _ redemptions.CreditGateway = (*Adapter)(nil)
