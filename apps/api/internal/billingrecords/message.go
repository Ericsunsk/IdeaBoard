package billingrecords

// Backend-owned user-facing copy for this package. Each constant
// is the single source of truth for one rendered message; call
// sites reference these names so copy changes stay in one place.
// Values are byte-identical to the previously inline literals.
const (
	msgDescribeLedgerRecord  = "Idea 消费" // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord2 = "欢迎奖励"    // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord3 = "兑换奖励"    // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord4 = "Idea 充值" // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord5 = "系统赠送"    // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord6 = "奖励"      // DescribeLedgerRecord @ service.go
	msgDescribeLedgerRecord7 = "系统调整"    // DescribeLedgerRecord @ service.go
)
