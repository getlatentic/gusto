package web

// matchOwner does nothing on Windows, where there is no agent user.
func matchOwner(path, root string) {}
