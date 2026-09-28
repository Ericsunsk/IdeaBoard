package billingrecords

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, input ListInput) (Page, error) {
	if s == nil || s.repository == nil {
		return Page{}, ErrRepositoryNotConfigured
	}
	if input.UserID <= 0 || input.BeforeSequence < 0 {
		return Page{}, ErrInvalidListInput
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultPageLimit
	}
	if limit < 1 || limit > MaxPageLimit {
		return Page{}, ErrInvalidPageLimit
	}

	ledgerRecords, err := s.repository.List(ctx, input.UserID, input.BeforeSequence, limit+1)
	if err != nil {
		return Page{}, err
	}
	page := Page{Records: make([]Record, 0, min(len(ledgerRecords), limit))}
	if len(ledgerRecords) > limit {
		page.NextBeforeSequence = ledgerRecords[limit-1].sequence
		ledgerRecords = ledgerRecords[:limit]
	}
	for _, ledgerRecord := range ledgerRecords {
		record, err := recordFromLedger(ledgerRecord)
		if err != nil {
			return Page{}, err
		}
		page.Records = append(page.Records, record)
	}
	return page, nil
}

func recordFromLedger(raw ledgerRecord) (Record, error) {
	if raw.amount > MaxSafeInteger || raw.availableAfter > MaxSafeInteger {
		return Record{}, fmt.Errorf("billing records service: record %s: %w", raw.recordID, ErrUnsafeNumber)
	}
	kind, title, delta, err := describeLedgerRecord(raw)
	if err != nil {
		return Record{}, fmt.Errorf("billing records service: record %s: %w", raw.recordID, err)
	}
	return Record{
		RecordID:              raw.recordID,
		OccurredAt:            raw.occurredAt,
		Kind:                  kind,
		Title:                 title,
		CreditDelta:           delta,
		AvailableCreditsAfter: raw.availableAfter,
	}, nil
}

func describeLedgerRecord(raw ledgerRecord) (Kind, string, int64, error) {
	switch raw.commandType {
	case ledgerCommandCapture:
		if raw.businessReferenceType != ledgerReferenceTask {
			return KindUsage, msgDescribeLedgerRecord, -raw.amount, nil
		}
		displayName := strings.TrimSpace(raw.snapshotDisplayName.String)
		if !raw.snapshotDisplayName.Valid || displayName == "" {
			return "", "", 0, ErrMissingSnapshotName
		}
		return KindAIUsage, displayName, -raw.amount, nil
	case ledgerCommandGrant:
		switch raw.source {
		case ledgerSourceWelcomeGrant:
			return KindReward, msgDescribeLedgerRecord2, raw.amount, nil
		case ledgerSourceRewardRedemption:
			return KindReward, msgDescribeLedgerRecord3, raw.amount, nil
		case ledgerSourceCreditPurchase:
			return KindTopUp, msgDescribeLedgerRecord4, raw.amount, nil
		case ledgerSourceAdminGrant:
			return KindSystemGrant, msgDescribeLedgerRecord5, raw.amount, nil
		default:
			if raw.actorType == ledgerActorAdmin {
				return KindSystemGrant, msgDescribeLedgerRecord5, raw.amount, nil
			}
			return KindReward, msgDescribeLedgerRecord6, raw.amount, nil
		}
	case ledgerCommandDeduction:
		return KindSystemAdjustment, msgDescribeLedgerRecord7, -raw.amount, nil
	case ledgerCommandCorrection:
		switch raw.originalCommandType.Value {
		case ledgerCommandGrant:
			return KindSystemAdjustment, msgDescribeLedgerRecord7, -raw.amount, nil
		case ledgerCommandDeduction:
			return KindSystemAdjustment, msgDescribeLedgerRecord7, raw.amount, nil
		default:
			return "", "", 0, ErrUndisplayableCorrection
		}
	default:
		return "", "", 0, fmt.Errorf("%w: %q", ErrUnsupportedCommandType, raw.commandType)
	}
}
