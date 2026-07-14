package billingrecords

import (
	"context"
	"errors"
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
		return Page{}, errors.New("billing records service: repository not configured")
	}
	if input.UserID <= 0 || input.BeforeSequence < 0 {
		return Page{}, errors.New("billing records service: invalid list input")
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultPageLimit
	}
	if limit < 1 || limit > MaxPageLimit {
		return Page{}, errors.New("billing records service: invalid page limit")
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
			return KindUsage, "Idea 消费", -raw.amount, nil
		}
		displayName := strings.TrimSpace(raw.snapshotDisplayName.String)
		if !raw.snapshotDisplayName.Valid || displayName == "" {
			return "", "", 0, errors.New("AI usage is missing its immutable Task Billing Snapshot name")
		}
		return KindAIUsage, displayName, -raw.amount, nil
	case ledgerCommandGrant:
		switch raw.source {
		case ledgerSourceWelcomeGrant:
			return KindReward, "欢迎奖励", raw.amount, nil
		case ledgerSourceRewardRedemption:
			return KindReward, "兑换奖励", raw.amount, nil
		case ledgerSourceRewardCampaign:
			return KindReward, "活动奖励", raw.amount, nil
		case ledgerSourceCreditPurchase:
			return KindTopUp, "Idea 充值", raw.amount, nil
		case ledgerSourceAdminGrant:
			return KindSystemGrant, "系统赠送", raw.amount, nil
		default:
			if raw.actorType == ledgerActorAdmin {
				return KindSystemGrant, "系统赠送", raw.amount, nil
			}
			return KindReward, "奖励", raw.amount, nil
		}
	case ledgerCommandDeduction:
		return KindSystemAdjustment, "系统调整", -raw.amount, nil
	case ledgerCommandCorrection:
		switch raw.originalCommandType.Value {
		case ledgerCommandGrant:
			return KindSystemAdjustment, "系统调整", -raw.amount, nil
		case ledgerCommandDeduction:
			return KindSystemAdjustment, "系统调整", raw.amount, nil
		default:
			return "", "", 0, errors.New("correction does not reference a displayable adjustment")
		}
	default:
		return "", "", 0, fmt.Errorf("unsupported command type %q", raw.commandType)
	}
}
