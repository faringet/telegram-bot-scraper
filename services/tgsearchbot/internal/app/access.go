package app

func (a *App) isAllowed(userID int64, chatID int64) bool {
	if a == nil || a.cfg == nil {
		return false
	}

	allowedUsers := a.cfg.Access.AllowedUserIDs
	allowedChats := a.cfg.Access.AllowedChatIDs

	if len(allowedUsers) == 0 && len(allowedChats) == 0 {
		return true
	}

	userOK := len(allowedUsers) == 0 || containsInt64(allowedUsers, userID)
	chatOK := len(allowedChats) == 0 || containsInt64(allowedChats, chatID)

	return userOK && chatOK
}

func containsInt64(items []int64, want int64) bool {
	for _, v := range items {
		if v == want {
			return true
		}
	}
	return false
}
