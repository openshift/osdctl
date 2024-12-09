package utils

import (
	"testing"
)

func TestGenerateQuery(t *testing.T) {
	tests := []struct {
		name              string
		clusterIdentifier string
		want              string
	}{
		{
			name:              "valid internal ID",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64ji",
			want:              "(id = '261kalm3uob0vegg1c7h9o7r5k9t64ji')",
		},
		{
			name:              "valid wrong internal ID with upper case",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64jI",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64jI')",
		},
		{
			name:              "valid wrong internal ID too short",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64j",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64j')",
		},
		{
			name:              "valid wrong internal ID too long",
			clusterIdentifier: "261kalm3uob0vegg1c7h9o7r5k9t64jix",
			want:              "(display_name like '261kalm3uob0vegg1c7h9o7r5k9t64jix')",
		},
		{
			name:              "valid external ID",
			clusterIdentifier: "c1f562af-fb22-42c5-aa07-6848e1eeee9c",
			want:              "(external_id = 'c1f562af-fb22-42c5-aa07-6848e1eeee9c')",
		},
		{
			name:              "valid display name",
			clusterIdentifier: "hs-mc-773jpgko0",
			want:              "(display_name like 'hs-mc-773jpgko0')",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateQuery(tt.clusterIdentifier); got != tt.want {
				t.Errorf("GenerateQuery(%s) = %v, want %v", tt.clusterIdentifier, got, tt.want)
			}
		})
	}
}
