package maxpatrol

type OrderBy struct {
	Field     string `json:"field"`
	SortOrder string `json:"sortOrder"`
}

// AccountUserinfo is returned by GET /api/account/userinfo.
type AccountUserinfo struct {
	UserID          string   `json:"userId"`
	UserName        string   `json:"userName"`
	FirstName       *string  `json:"firstName"`
	LastName        *string  `json:"lastName"`
	Roles           []string `json:"roles"`
	PasswordExpired bool     `json:"passwordExpired"`
}
