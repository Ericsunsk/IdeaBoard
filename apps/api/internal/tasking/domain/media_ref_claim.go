package domain

// MediaRefClaim is the immutable value object for claiming one prepared media
// asset into a task submission transaction. It replaces the unnamed
// (taskID, userID, assetID, preparedRefID, role, inputIndex) parameter group
// previously threaded across the submission and persistence seams.
type MediaRefClaim struct {
	TaskID        string
	OwnerUserID   int64
	AssetID       int64
	PreparedRefID string
	Role          string
	InputIndex    int
}
