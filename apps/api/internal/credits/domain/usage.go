package domain

// UsageFilter narrows account usage to a consumption workspace and/or the real
// initiating member. Zero values mean all workspaces or members respectively;
// only the authorization boundary may choose an unrestricted filter.
type UsageFilter struct {
	WorkspaceID int64
	ActorUserID int64
}

func (f UsageFilter) Valid() bool { return f.WorkspaceID >= 0 && f.ActorUserID >= 0 }
