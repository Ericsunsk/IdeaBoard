package billingrecords

type ledgerCommand string

const (
	ledgerCommandGrant      ledgerCommand = "grant"
	ledgerCommandDeduction  ledgerCommand = "deduction"
	ledgerCommandCapture    ledgerCommand = "capture"
	ledgerCommandCorrection ledgerCommand = "correction"
)

type ledgerActorType string

const ledgerActorAdmin ledgerActorType = "admin"

type ledgerSource string

const (
	ledgerSourceWelcomeGrant     ledgerSource = "welcome_grant"
	ledgerSourceRewardRedemption ledgerSource = "reward_redemption"
	ledgerSourceCreditPurchase   ledgerSource = "credit_purchase"
	ledgerSourceAdminGrant       ledgerSource = "admin_grant"
)

type ledgerReferenceType string

const ledgerReferenceTask ledgerReferenceType = "task"

type nullableLedgerCommand struct {
	Value ledgerCommand
	Valid bool
}
