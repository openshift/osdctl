package utils

type SilenceID struct {
	ID string `json:"id"`
}

type SilenceMatchers struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SilenceStatus struct {
	State string `json:"state"`
}

type Silence struct {
	ID        string            `json:"id"`
	Matchers  []SilenceMatchers `json:"matchers"`
	Status    SilenceStatus     `json:"status"`
	Comment   string            `json:"comment"`
	CreatedBy string            `json:"createdBy"`
	EndsAt    string            `json:"endsAt"`
	StartsAt  string            `json:"startsAt"`
}
