package docker

// SupportsTemplate marks docker as a provider that starts sandboxes from
// an image.
func (*Provider) SupportsTemplate() {}

// SetDefaultTemplate sets the image used when a create names none. An empty
// name restores the image of the shell agent.
func (p *Provider) SetDefaultTemplate(name string) error {
	if err := validateTemplate(name); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.template = name
	return nil
}

// templateFor returns the image a create uses, or "" for the default.
func (p *Provider) templateFor(requested string) string {
	if requested != "" {
		return requested
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.template
}
