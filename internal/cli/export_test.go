package cli

// SetOpenBrowserForTest replaces the browser opener until the returned function runs.
func SetOpenBrowserForTest(fn func(string) error) func() {
	prev := openBrowserFn
	if fn == nil {
		fn = func(string) error { return nil }
	}
	openBrowserFn = fn
	return func() { openBrowserFn = prev }
}
