package logsafe

// RequestID is RequestRef(incoming, mint).String(): the normalised id for the
// response header. See RequestRef for the rules.
func RequestID(incoming string, mint func() string) string {
	return RequestRef(incoming, mint).String()
}
