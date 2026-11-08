package atomicfile

// syncDir does nothing on Windows, which has no way to fsync a directory: NTFS keeps a rename in its journal.
func syncDir(string) error { return nil }
