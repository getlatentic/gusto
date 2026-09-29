package changelog

// HashOf lets a test do what a forger would: recompute a line's hash.
func HashOf(e Event) (string, error) { return hashOf(e) }
