package main

// loginShellPath is empty on Windows, where a program the person starts is given their own PATH.
func loginShellPath() string { return "" }
