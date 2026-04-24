package boards

type UpsertBoardRequest struct {
	Name      string      `json:"name" binding:"required"`
	State     interface{} `json:"state"`
	CreatedAt *int64      `json:"created_at"`
	UpdatedAt *int64      `json:"updated_at"`
}
