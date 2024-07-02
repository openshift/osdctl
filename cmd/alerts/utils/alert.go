package utils

// Labels represents a set of labels associated with an alert.
type AlertLabels struct {
	Alertname string `json:"alertname"`
	Severity  string `json:"severity"`
}

// Status represents a set of state associated with an alert.
type AlertStatus struct {
	State string `json:"state"`
}

// Annotations represents a set of summary/description associated with an alert.
type AlertAnnotations struct {
	Summary string `json:"summary"`
}

// Alert represents a set of above declared struct Labels,Status and annoataions
type Alert struct {
	Labels      AlertLabels      `json:"labels"`
	Status      AlertStatus      `json:"status"`
	Annotations AlertAnnotations `json:"annotations"`
}
