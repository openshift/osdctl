package cmd

type recordingPrompt struct {
	confirmed bool
	messages  []string
}

func (p *recordingPrompt) Confirm(message string) bool {
	p.messages = append(p.messages, message)
	return p.confirmed
}
