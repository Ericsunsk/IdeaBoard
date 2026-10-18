package domain

type LedgerCommand string

const (
	LedgerCommandGrant      LedgerCommand = "grant"
	LedgerCommandDeduction  LedgerCommand = "deduction"
	LedgerCommandCapture    LedgerCommand = "capture"
	LedgerCommandCorrection LedgerCommand = "correction"
)

type LedgerActorType string

// LedgerActorAdmin mirrors credits/domain.ActorAdmin ("admin") by value, not
// by import: billingrecords is a read-model over ledger snapshots and must
// not depend on the credits write domain. Keep the two spellings identical;
// a shared constant would couple the read model to the writer.
const LedgerActorAdmin LedgerActorType = "admin"

type LedgerSource string

const (
	LedgerSourceWelcomeGrant     LedgerSource = "welcome_grant"
	LedgerSourceRewardRedemption LedgerSource = "reward_redemption"
	LedgerSourceCreditPurchase   LedgerSource = "credit_purchase"
	LedgerSourceAdminGrant       LedgerSource = "admin_grant"
)

type LedgerReferenceType string

const LedgerReferenceTask LedgerReferenceType = "task"

type NullableLedgerCommand struct {
	Value LedgerCommand
	Valid bool
}
