package notifications

// ListFilter selects a notification feed before pagination. The default
// feed excludes archived rows; archived rows are available explicitly.
type ListFilter struct {
	Status   string
	Category string
}

func (f ListFilter) matches(n *Notification) bool {
	if f.Status == "" {
		if n.Status == StatusArchived {
			return false
		}
	} else if string(n.Status) != f.Status {
		return false
	}
	switch f.Category {
	case "invites":
		switch n.Type {
		case TypeWorkspaceInvite, TypeWorkspaceMemberJoined, TypeWorkspaceInviteDeclined,
			TypeOrganizationInvite, TypeOrganizationMemberJoined, TypeOrganizationInviteDeclined,
			TypeStudioHandover:
			return true
		}
		return false
	case "billing":
		return n.Type == TypeBillingCreditLow || n.Type == TypeBillingRechargeSuccess
	default:
		return true
	}
}
