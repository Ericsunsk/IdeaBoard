package application

import (
	"context"
	"fmt"
	"ideaboard/api/internal/platform/errorrender"
	"strings"

	billingrecordsdomain "ideaboard/api/internal/billingrecords/domain"
	billingrecordsports "ideaboard/api/internal/billingrecords/ports"
)

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) List(ctx context.Context, input billingrecordsports.ListInput) (billingrecordsports.Page, error) {
	if s == nil || s.repository == nil {
		return billingrecordsports.Page{}, billingrecordsports.ErrRepositoryNotConfigured
	}
	if input.BillingAccountID <= 0 || input.BeforeSequence < 0 || input.WorkspaceID < 0 || input.ActorUserID < 0 {
		return billingrecordsports.Page{}, billingrecordsports.ErrInvalidListInput
	}
	limit := input.Limit
	if limit == 0 {
		limit = billingrecordsports.DefaultPageLimit
	}
	if limit < 1 || limit > billingrecordsports.MaxPageLimit {
		return billingrecordsports.Page{}, billingrecordsports.ErrInvalidPageLimit
	}

	ledgerRecords, err := s.repository.List(ctx, input, limit+1)
	if err != nil {
		return billingrecordsports.Page{}, err
	}
	page := billingrecordsports.Page{Records: make([]billingrecordsports.Record, 0, min(len(ledgerRecords), limit))}
	if len(ledgerRecords) > limit {
		page.NextBeforeSequence = ledgerRecords[limit-1].sequence
		ledgerRecords = ledgerRecords[:limit]
	}
	for _, ledgerRecord := range ledgerRecords {
		record, err := recordFromLedger(ledgerRecord)
		if err != nil {
			return billingrecordsports.Page{}, err
		}
		if input.WorkspaceID != 0 || input.ActorUserID != 0 {
			record.AvailableCreditsAfter = nil
		}
		page.Records = append(page.Records, record)
	}
	return page, nil
}

func recordFromLedger(raw ledgerRecord) (billingrecordsports.Record, error) {
	if raw.amount > billingrecordsports.MaxSafeInteger || raw.availableAfter > billingrecordsports.MaxSafeInteger {
		return billingrecordsports.Record{}, fmt.Errorf("billing records service: record %s: %w", raw.recordID, billingrecordsports.ErrUnsafeNumber)
	}
	kind, title, delta, err := describeLedgerRecord(raw)
	if err != nil {
		return billingrecordsports.Record{}, fmt.Errorf("billing records service: record %s: %w", raw.recordID, err)
	}
	return billingrecordsports.Record{
		RecordID:              raw.recordID,
		OccurredAt:            raw.occurredAt,
		Kind:                  kind,
		Title:                 title,
		CreditDelta:           delta,
		AvailableCreditsAfter: &raw.availableAfter,
	}, nil
}

func describeLedgerRecord(raw ledgerRecord) (billingrecordsports.Kind, string, int64, error) {
	switch raw.commandType {
	case billingrecordsdomain.LedgerCommandCapture:
		if raw.businessReferenceType != billingrecordsdomain.LedgerReferenceTask {
			return billingrecordsports.KindUsage, errorrender.MsgDescribeLedgerRecord, -raw.amount, nil
		}
		displayName := strings.TrimSpace(raw.snapshotDisplayName.String)
		if !raw.snapshotDisplayName.Valid || displayName == "" {
			return "", "", 0, billingrecordsports.ErrMissingSnapshotName
		}
		return billingrecordsports.KindAIUsage, displayName, -raw.amount, nil
	case billingrecordsdomain.LedgerCommandGrant:
		switch raw.source {
		case billingrecordsdomain.LedgerSourceWelcomeGrant:
			return billingrecordsports.KindReward, errorrender.MsgDescribeLedgerRecord2, raw.amount, nil
		case billingrecordsdomain.LedgerSourceRewardRedemption:
			return billingrecordsports.KindReward, errorrender.MsgDescribeLedgerRecord3, raw.amount, nil
		case billingrecordsdomain.LedgerSourceCreditPurchase:
			return billingrecordsports.KindTopUp, errorrender.MsgDescribeLedgerRecord4, raw.amount, nil
		case billingrecordsdomain.LedgerSourceAdminGrant:
			return billingrecordsports.KindSystemGrant, errorrender.MsgDescribeLedgerRecord5, raw.amount, nil
		default:
			if raw.actorType == billingrecordsdomain.LedgerActorAdmin {
				return billingrecordsports.KindSystemGrant, errorrender.MsgDescribeLedgerRecord5, raw.amount, nil
			}
			return billingrecordsports.KindReward, errorrender.MsgDescribeLedgerRecord6, raw.amount, nil
		}
	case billingrecordsdomain.LedgerCommandDeduction:
		return billingrecordsports.KindSystemAdjustment, errorrender.MsgDescribeLedgerRecord7, -raw.amount, nil
	case billingrecordsdomain.LedgerCommandCorrection:
		switch raw.originalCommandType.Value {
		case billingrecordsdomain.LedgerCommandGrant:
			return billingrecordsports.KindSystemAdjustment, errorrender.MsgDescribeLedgerRecord7, -raw.amount, nil
		case billingrecordsdomain.LedgerCommandDeduction:
			return billingrecordsports.KindSystemAdjustment, errorrender.MsgDescribeLedgerRecord7, raw.amount, nil
		default:
			return "", "", 0, billingrecordsports.ErrUndisplayableCorrection
		}
	default:
		return "", "", 0, fmt.Errorf("%w: %q", billingrecordsports.ErrUnsupportedCommandType, raw.commandType)
	}
}
